package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"net"

	"golang.org/x/net/context"

	"github.com/antage/eventsource"
	"github.com/ericpai/msops"
	"github.com/golang/glog"
	"github.com/laincloud/lainlet/client"
)

type MySQLMonitor struct {
	es           *eventsource.EventSource
	master       string
	slave        map[string]interface{}
	standby      string
	unregistered map[string]interface{}
	newConnChan  chan string
	newEventChan chan map[string]interface{}
	getReqChan   chan GetRequest
	patchReqChan chan PatchRequest
}

type ProcInstance struct {
	InstanceNo int    `json:"InstanceNo"`
	Port       int    `json:"Port"`
	ProcName   string `json:"ProcName"`
}

type AppProcs struct {
	Procs []ProcInstance `json:"proc"`
}

type AuthConfInfo struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

const (
	sseID          = "1"
	sseInit        = "init"
	sseUpdate      = "update"
	reportFormat   = "%s.%s.%s.%s %d %d\n"
	secretFileName = "conf/secret.conf"
	dbaUser        = "dba"
	replUser       = "repl"
	masterConfig   = "/var/lib/monitor.conf/master"
	slaveConfig    = "/var/lib/monitor.conf/slave"
	standbyConfig  = "/var/lib/monitor.conf/standby"
	roleMaster     = "master"
	roleSlave      = "slave"
	reportTime     = time.Minute
	inspectTime    = 3 * time.Second
	CooldownTime   = 3 * time.Second
)

const (
	MonitorPort     = "6033"
	MonitorLocation = "/servers"
)

var (
	lainDomain      = os.Getenv("LAIN_DOMAIN")
	lainAppName     = os.Getenv("LAIN_APPNAME")
	graphiteAddress = net.JoinHostPort("graphite.lain", os.Getenv("GRAPHITE_PORT"))
	lainletClient   = client.New(net.JoinHostPort("lainlet.lain", os.Getenv("LAINLET_PORT")))
	procWatcherURL  = fmt.Sprintf("/v2/procwatcher?appname=%s", lainAppName)

	graphiteKeyDomain  = strings.Replace(lainDomain, ".", "_", -1)
	graphiteKeyAppName = strings.Replace(lainAppName, ".", "_", -1)
	placeHolder        = new(interface{})
	webPrefix          = getWebPrefix(lainAppName)

	SSORedirectURI = fmt.Sprintf("http://%s.%s/", webPrefix, lainDomain)
	ConsoleAuthURL = fmt.Sprintf("http://console.%s/api/v1/repos/%s/roles/", lainDomain, lainAppName)

	connParam = map[string]string{
		"charset": "utf8",
		"timeout": "1s",
	}
	sysUsers = []string{dbaUser, replUser, "root", "system user"}

	SecretConf = getSecretConf()

	msMonitor MySQLMonitor
)

// Start starts the main goroutine of monitor
func Start() {

	settings := &eventsource.Settings{
		IdleTimeout:    6 * time.Hour,
		CloseOnTimeout: true,
		Timeout:        3 * time.Second,
	}
	eventsource := eventsource.New(settings, nil)
	msMonitor = MySQLMonitor{
		es:           &eventsource,
		slave:        make(map[string]interface{}),
		unregistered: make(map[string]interface{}),
		newConnChan:  make(chan string),
		newEventChan: make(chan map[string]interface{}),
		getReqChan:   make(chan GetRequest),
		patchReqChan: make(chan PatchRequest),
	}
	defer (*(msMonitor.es)).Close()

	msMonitor.loadConfig()
	http.Handle(MonitorLocation, *(msMonitor.es))
	go msMonitor.listenLainletEvent()
	go msMonitor.run()
	glog.Fatal(http.ListenAndServe(net.JoinHostPort("", MonitorPort), msMonitor))
}

// ServeHTTP sends a singal to monitor sending init event to the new proxy
func (monitor MySQLMonitor) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	(*(monitor.es)).ServeHTTP(rw, req)
	monitor.newConnChan <- req.RemoteAddr
}

func (monitor *MySQLMonitor) run() {
	prevData := monitor.inspect()
	reportTick := time.Tick(reportTime)
	inspectTick := time.Tick(inspectTime)
	for {
		select {
		case portalEndpoint := <-monitor.newConnChan:
			glog.V(2).Infof("Portal %s connnected to monitor", portalEndpoint)
			(*(monitor.es)).SendEventMessage(prevData, sseInit, sseID)
			glog.V(2).Infof("Send data: %s", string(prevData))
		case newInstList := <-monitor.newEventChan:
			monitor.updateServersList(newInstList)
			newData := monitor.inspect()
			if prevData != newData {
				prevData = newData
				glog.V(2).Info("Server list is updated")
				(*(monitor.es)).SendEventMessage(prevData, sseUpdate, sseID)
				glog.V(2).Infof("Send data: %s", string(prevData))
			}
		case req := <-monitor.getReqChan:
			monitor.handleGet(req)
		case req := <-monitor.patchReqChan:
			monitor.handlePatch(req)
		case <-inspectTick:
			newData := monitor.inspect()
			if prevData != newData {
				prevData = newData
				glog.V(2).Info("Inspect finished, the cluster status is changed")
				(*(monitor.es)).SendEventMessage(prevData, sseUpdate, sseID)
				glog.V(2).Infof("Send data: %s", string(prevData))
			}
		case <-reportTick:
			monitor.report()
		}
		glog.Flush()
	}
}

func (monitor *MySQLMonitor) inspect() string {
	roleEndpoints := make(map[string][]string)

	roleEndpoints[roleMaster] = make([]string, 0, 1)

	if msops.CheckInstance(monitor.master) == msops.InstanceOK {
		roleEndpoints[roleMaster] = append(roleEndpoints[roleMaster], monitor.master)
	}

	roleEndpoints[roleSlave] = make([]string, 0, len(monitor.slave))

	for endpoint := range monitor.slave {
		if st := msops.CheckReplication(endpoint, monitor.master); st == msops.ReplicationOK || st == msops.ReplicationSyning {
			roleEndpoints[roleSlave] = append(roleEndpoints[roleSlave], endpoint)
		}
	}
	jsonStr, _ := json.Marshal(roleEndpoints)
	monitor.saveConfig()
	return string(jsonStr)
}

func (monitor *MySQLMonitor) updateServersList(newInstList map[string]interface{}) {
	if monitor.master != "" {
		if _, exist := newInstList[monitor.master]; !exist {
			// If master endpoint is lost, we are dead
			glog.Errorf("Can't find master endpoint %s. Unregistered", monitor.master)
			msops.Unregister(monitor.master)
			monitor.master = ""
		} else {
			delete(newInstList, monitor.master)
		}
	}

	for endpoint := range monitor.slave {
		if _, exist := newInstList[endpoint]; !exist {
			glog.V(1).Infof("Slave %s is missed. Unregistered", endpoint)
			msops.Unregister(endpoint)
			delete(monitor.slave, endpoint)
		} else {
			delete(newInstList, endpoint)
		}
	}

	if monitor.standby != "" {
		if _, exist := newInstList[monitor.standby]; !exist {
			glog.Infof("Standby %s is missed", monitor.standby)
			msops.Unregister(monitor.standby)
			monitor.standby = ""
		} else {
			delete(newInstList, monitor.standby)
		}
	}

	for endpoint := range monitor.unregistered {
		if _, exist := newInstList[endpoint]; !exist {
			glog.V(1).Infof("Unregistered %s is missed", endpoint)
			delete(monitor.unregistered, endpoint)
		} else {
			delete(newInstList, endpoint)
		}
	}
	for newEndpoint := range newInstList {
		monitor.unregistered[newEndpoint] = placeHolder
	}
}

func (monitor *MySQLMonitor) listenLainletEvent() {
	endpointList := make(map[string]interface{})
	for {
		ch, err := lainletClient.Watch(procWatcherURL, context.Background())
		if err != nil {
			glog.Errorf("Goroutine listenLainletEvent connects to lainlet failed: %s", err.Error())
			time.Sleep(CooldownTime)
			continue
		}
		for event := range ch {
			var appProcs []AppProcs
			if err := json.Unmarshal(event.Data, &appProcs); err != nil {
				glog.Errorf("Unmarshal event error: %s", err.Error())
				time.Sleep(CooldownTime)
				continue
			}
			tmpList := make(map[string]interface{})
			for _, appProc := range appProcs {
				for _, instance := range appProc.Procs {
					if instance.ProcName == "mysql-server" {
						tmpList[fmt.Sprintf("%s-%d:%d", instance.ProcName, instance.InstanceNo, instance.Port)] = placeHolder
					}
				}
			}
			if !reflect.DeepEqual(tmpList, endpointList) {
				monitor.newEventChan <- tmpList
				endpointList = tmpList
			}
			time.Sleep(CooldownTime)
		}
		time.Sleep(CooldownTime)
	}
}

func (monitor *MySQLMonitor) report() {
	graphiteConf := make(map[string]string)
	if data, err := GetLainConf("features/graphite"); err != nil {
		glog.Errorf("Get graphite feature failed: %s", err.Error())
		return
	} else if err := json.Unmarshal(data, &graphiteConf); err != nil {
		glog.Errorf("Unmarshal graphite feature failed: %s", err.Error())
		return
	} else if needReport, _ := strconv.ParseBool(graphiteConf["features/graphite"]); !needReport {
		return
	}

	conn, err := net.DialTimeout("tcp", graphiteAddress, time.Second*2)
	if err != nil {
		glog.Errorf("Dial %s failed: %s", graphiteAddress, err.Error())
		return
	}
	defer conn.Close()
	sendData := strings.Join(monitor.getReportData(), "")
	if _, err = conn.Write([]byte(sendData)); err != nil {
		glog.Errorf("Send report data failed: %s", err.Error())
	}
	glog.Flush()
}

func (monitor *MySQLMonitor) getReportData() []string {
	var data []string
	data = append(data, prepareReportData(monitor.master)...)
	for endpoint := range monitor.slave {
		data = append(data, prepareReportData(endpoint)...)
	}
	data = append(data, prepareReportData(monitor.standby)...)
	return data
}

//loadConfig loads role information from local files
func (monitor *MySQLMonitor) loadConfig() {
	if data, err := load(masterConfig); err != nil {
		glog.Errorf("Load master config failed: %s", err.Error())
	} else if len(data) == 1 {
		monitor.master = data[0]
		if err := msops.Register(monitor.master, dbaUser, SecretConf["dba_passwd"], replUser, SecretConf["repl_passwd"], connParam); err != nil {
			glog.Errorf("Register master failed: %s", err.Error())
		}
	}

	if data, err := load(slaveConfig); err != nil {
		glog.Errorf("Load slave config failed: %s", err.Error())
	} else {
		for _, endpoint := range data {
			monitor.slave[endpoint] = placeHolder
			if err := msops.Register(endpoint, dbaUser, SecretConf["dba_passwd"], replUser, SecretConf["repl_passwd"], connParam); err != nil {
				glog.Errorf("Register slave failed: %s", err.Error())
			}
		}
	}

	if data, err := load(standbyConfig); err != nil {
		glog.Errorf("Load standby config failed: %s", err.Error())
	} else if len(data) == 1 {
		monitor.standby = data[0]
		if err := msops.Register(monitor.standby, dbaUser, SecretConf["dba_passwd"], replUser, SecretConf["repl_passwd"], connParam); err != nil {
			glog.Errorf("Register standby failed: %s", err.Error())
		}
	}

	glog.Flush()
}

//saveConfig saves role information to local files
func (monitor *MySQLMonitor) saveConfig() {
	if err := save(monitor.master, masterConfig); err != nil {
		glog.Errorf("Save master config failed: %s", err.Error())
	}
	slaveArr := make([]string, 0, len(monitor.slave))
	for endpoint := range monitor.slave {
		slaveArr = append(slaveArr, endpoint)
	}
	if err := save(strings.Join(slaveArr, "\n"), slaveConfig); err != nil {
		glog.Errorf("Save slave config failed: %s", err.Error())
	}
	if err := save(monitor.standby, standbyConfig); err != nil {
		glog.Errorf("Save standby config failed: %s", err.Error())
	}

	glog.Flush()
}

// handleGet handles GET requests from web users
func (monitor *MySQLMonitor) handleGet(req GetRequest) {
	resp := GetResponse{}
	switch req.RequestType {
	case GetAllOverview:
		resp.Data, resp.Code, resp.Err = getAllOverview()
	case GetOneDetails:
		resp.Data, resp.Code, resp.Err = getOneDetails(req.Params["endpoint"])
	}
	req.ResponseChan <- resp
}

// handleGet handles PATCH requests from web users
func (monitor *MySQLMonitor) handlePatch(req PatchRequest) {
	resp := PatchResponse{}
	_, isSlave := monitor.slave[req.Endpoint]
	_, isUnregistered := monitor.unregistered[req.Endpoint]
	if monitor.master != req.Endpoint && isSlave && monitor.standby != req.Endpoint && isUnregistered {
		resp.Code, resp.Err = http.StatusNotFound, fmt.Errorf("%s is not an invalid instance", req.Endpoint)
	} else {
		switch req.Action {
		case ActionActive:
			resp.Code, resp.Err = active(req.Endpoint)
		case ActionDetach:
			resp.Code, resp.Err = detach(req.Endpoint)
		case ActionPause:
			resp.Code, resp.Err = pause(req.Endpoint)
		case ActionRegisterMaster, ActionRegisterSlave, ActionRegisterStandby:
			if resp.Code, resp.Err = register(req.Endpoint, req.Action); resp.Err == nil {
				monitor.saveConfig()
			}
		case ActionResume:
			resp.Code, resp.Err = resume(req.Endpoint)
		case ActionSwtich:
			if resp.Code, resp.Err = switchToMaster(req.Endpoint); resp.Err == nil {
				monitor.saveConfig()
			}
		case ActionUnregister:
			if resp.Code, resp.Err = unregister(req.Endpoint); resp.Err == nil {
				monitor.saveConfig()
			}
		}
	}
	req.ResponseChan <- resp
}

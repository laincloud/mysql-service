package monitor

import (
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strconv"

	"github.com/ericpai/msops"
)

type PatchRequest struct {
	Action       PatchAction
	Endpoint     string
	ResponseChan chan PatchResponse
}

type PatchResponse struct {
	Err  error
	Code int
}

type GetRequest struct {
	RequestType  GetType
	Params       map[string]string
	ResponseChan chan GetResponse
}

type GetResponse struct {
	Data []byte
	Err  error
	Code int
}

type InstanceView struct {
	Addr                  string
	Port                  string
	Role                  string
	InstanceStatusText    string
	ReplicationStatusText string
	AllowedActions        []string
	ProcessesList         []map[string]string
	SlaveStatusList       map[string]string
	PerformanceStatusList map[string]string
}

type InstanceViewSorter []InstanceView

func (svs InstanceViewSorter) Len() int {
	return len(svs)
}

func (svs InstanceViewSorter) Swap(i, j int) {
	svs[i], svs[j] = svs[j], svs[i]
}

func (svs InstanceViewSorter) Less(i, j int) bool {
	return svs[i].Addr < svs[j].Addr || (svs[i].Addr == svs[j].Addr && svs[i].Port < svs[j].Port)
}

// Patch receives a PatchRequest and sends to monitor to handle
func Patch(req PatchRequest) {
	msMonitor.patchReqChan <- req
}

// Get receives a GetRequest and sends to monitor to handle
func Get(req GetRequest) {
	msMonitor.getReqChan <- req
}

func getOneDetails(endpoint string) ([]byte, int, error) {
	var instModel InstanceModel
	var err error
	var data []byte
	var code int
	var slaveStatus msops.SlaveStatus
	instModel, code, err = getInstance(endpoint)
	if err != nil {
		return data, code, err
	}
	instView := getInstaceViewFromModel(instModel)
	processList, _ := msops.GetProcessList(endpoint)
	instView.ProcessesList = make([]map[string]string, 0, len(processList))
	for _, process := range processList {
		instProcess := make(map[string]string)
		instProcess["Id"] = strconv.Itoa(process.ID)
		instProcess["User"] = process.User
		instProcess["Host"] = process.Host
		instProcess["db"] = process.DB
		instProcess["Command"] = process.Command
		instProcess["Time"] = strconv.Itoa(process.Time)
		instProcess["State"] = process.State
		instProcess["Info"] = process.Info
		instView.ProcessesList = append(instView.ProcessesList, instProcess)
	}
	slaveStatus, err = msops.GetSlaveStatus(endpoint)
	instView.SlaveStatusList = make(map[string]string)
	if err == nil {
		instView.SlaveStatusList["Auto_Position"] = strconv.FormatBool(slaveStatus.AutoPosition)
		instView.SlaveStatusList["Exec_Master_Log_Pos"] = strconv.Itoa(slaveStatus.ExecMasterLogPos)
		instView.SlaveStatusList["Executed_Gtid_Set"] = slaveStatus.ExecutedGtidSet
		instView.SlaveStatusList["Last_Errno"] = strconv.Itoa(slaveStatus.LastErrno)
		instView.SlaveStatusList["Last_Error"] = slaveStatus.LastError
		instView.SlaveStatusList["Last_IO_Errno"] = strconv.Itoa(slaveStatus.LastIOErrno)
		instView.SlaveStatusList["Last_IO_Error"] = slaveStatus.LastIOError
		instView.SlaveStatusList["Last_IO_Error_Timestamp"] = slaveStatus.LastIOErrorTimestamp
		instView.SlaveStatusList["Last_SQL_Errno"] = strconv.Itoa(slaveStatus.LastSQLErrno)
		instView.SlaveStatusList["Last_SQL_Error"] = slaveStatus.LastSQLError
		instView.SlaveStatusList["Last_SQL_Error_Timestamp"] = slaveStatus.LastSQLErrorTimestamp
		instView.SlaveStatusList["Master_Host"] = slaveStatus.MasterHost
		instView.SlaveStatusList["Master_Log_File"] = slaveStatus.MasterLogFile
		instView.SlaveStatusList["Master_Port"] = strconv.Itoa(slaveStatus.MasterPort)
		instView.SlaveStatusList["Master_User"] = slaveStatus.MasterUser
		instView.SlaveStatusList["Read_Master_Log_Pos"] = strconv.Itoa(slaveStatus.ReadMasterLogPos)
		instView.SlaveStatusList["Relay_Log_File"] = slaveStatus.RelayLogFile
		instView.SlaveStatusList["Relay_Log_Pos"] = strconv.Itoa(slaveStatus.RelayLogPos)
		instView.SlaveStatusList["Relay_Log_Space"] = strconv.Itoa(slaveStatus.RelayLogSpace)
		instView.SlaveStatusList["Relay_Master_Log_File"] = slaveStatus.RelayMasterLogFile
		instView.SlaveStatusList["Seconds_Behind_Master"] = strconv.Itoa(slaveStatus.SecondsBehindMaster)
		instView.SlaveStatusList["Slave_IO_Running"] = slaveStatus.SlaveIORunning
		instView.SlaveStatusList["Slave_IO_State"] = slaveStatus.SlaveIOState
		instView.SlaveStatusList["Slave_SQL_Running"] = slaveStatus.SlaveSQLRunning
		instView.SlaveStatusList["Slave_SQL_Running_State"] = slaveStatus.SlaveSQLRunningState
	}
	instView.PerformanceStatusList, _ = msops.GetGlobalStatus(endpoint, "%")
	data, err = json.Marshal(instView)
	if err != nil {
		code = http.StatusInternalServerError
	} else {
		code = http.StatusOK
	}
	return data, code, err
}

func getAllOverview() ([]byte, int, error) {
	var instances []InstanceModel
	var err error
	var data []byte
	var code int
	instances, code, err = getAllInstances()
	if err != nil {
		return data, code, err
	}
	viewModels := make([]InstanceView, 0, len(instances))
	for _, inst := range instances {
		viewModels = append(viewModels, getInstaceViewFromModel(inst))
	}
	sort.Sort(InstanceViewSorter(viewModels))
	data, err = json.Marshal(viewModels)
	if err != nil {
		code = http.StatusInternalServerError
	} else {
		code = http.StatusOK
	}
	return data, code, err
}

func getInstaceViewFromModel(model InstanceModel) InstanceView {
	view := InstanceView{
		Role: model.Role,
		Addr: model.Addr,
		Port: model.Port,
	}
	// Set InstanceStatus view part
	switch model.InstanceStatus {
	case msops.InstanceOK:
		view.InstanceStatusText = "OK"
	case msops.InstanceERROR:
		view.InstanceStatusText = "ERROR"
	case msops.InstanceUnregistered:
		view.InstanceStatusText = "UNREGISTERED"
	}

	// Set ReplicationStatus view part
	switch model.ReplicationStatus {
	case msops.ReplicationError:
		view.ReplicationStatusText = "ERROR"
	case msops.ReplicationNone:
		view.ReplicationStatusText = "NONE"
	case msops.ReplicationOK:
		view.ReplicationStatusText = "OK"
	case msops.ReplicationPausing:
		view.ReplicationStatusText = "PAUSING"
	case msops.ReplicationSyning:
		view.ReplicationStatusText = "SYNING"
	case msops.ReplicationUnknown:
		view.ReplicationStatusText = "UNKNOWN"
	case msops.ReplicationWrongMaster:
		view.ReplicationStatusText = "WRONG MASTER"
	}

	// Set ActionStatus view part
	switch model.Role {
	case "Master":
		res, _ := msops.GetGlobalVariables(net.JoinHostPort(model.Addr, model.Port), "read_only")
		if res["read_only"] == "OFF" {
			view.AllowedActions = []string{string(ActionPause)}
		} else {
			view.AllowedActions = []string{string(ActionResume)}
		}
		if model.ReplicationStatus == msops.ReplicationNone {
			if msMonitor.standby != "" {
				view.AllowedActions = append(view.AllowedActions, string(ActionActive))
			}
		} else {
			view.AllowedActions = append(view.AllowedActions, string(ActionDetach))
		}
	case "Unregistered":
		view.AllowedActions = make([]string, 0)
		if msMonitor.master == "" {
			view.AllowedActions = append(view.AllowedActions, string(ActionRegisterMaster))
		} else {
			view.AllowedActions = append(view.AllowedActions, string(ActionRegisterSlave))
			if msMonitor.standby == "" {
				view.AllowedActions = append(view.AllowedActions, string(ActionRegisterStandby))
			}
		}

	default:
		view.AllowedActions = make([]string, 0)
		switch model.ReplicationStatus {
		case msops.ReplicationNone:
			view.AllowedActions = append(view.AllowedActions, string(ActionActive))
		case msops.ReplicationOK:
			view.AllowedActions = append(view.AllowedActions, string(ActionDetach), string(ActionPause), string(ActionSwtich))
		case msops.ReplicationSyning:
			view.AllowedActions = append(view.AllowedActions, string(ActionDetach), string(ActionPause), string(ActionSwtich))
		case msops.ReplicationPausing:
			view.AllowedActions = append(view.AllowedActions, string(ActionDetach), string(ActionResume))
		case msops.ReplicationWrongMaster:
			view.AllowedActions = append(view.AllowedActions, string(ActionDetach))
		}
		view.AllowedActions = append(view.AllowedActions, string(ActionUnregister))
	}
	return view
}

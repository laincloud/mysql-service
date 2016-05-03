package proxy

import (
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"encoding/json"

	"github.com/golang/glog"
	"github.com/laincloud/lainlet/client"
	"github.com/laincloud/mysql-service/monitor"
	"golang.org/x/net/context"
)

const (
	cooldownTime    = 3 * time.Second
	monitorProcName = "web-1"
)

var targetsLock sync.RWMutex

// MySQLProxy proxies clients' requests to mysql servers.
// The targets are thread-safe
type MySQLProxy struct {
	servicePort   int
	serviceMode   string // master or slave
	targets       []string
	roundrobinIdx int
}

// StartProxy starts a MySQLProxy listening in port and serving for mode(master|slave)
func StartProxy(port int, mode string) {
	rp := MySQLProxy{
		servicePort:   port,
		serviceMode:   mode,
		roundrobinIdx: -1,
	}
	//启动监听客户端连接的goroutine
	go rp.listenConnectRequest()
	glog.V(1).Infof("Start proxy. Server port: %d, mode: %s", port, mode)
	rp.getInfoFromMonitor()
}

func (rp *MySQLProxy) getInfoFromMonitor() {
	monitorClient := client.New(net.JoinHostPort(monitorProcName, monitor.MonitorPort))
	glog.V(1).Info("Connect to Monitor")
	for {
		ch, err := monitorClient.Watch(monitor.MonitorLocation, context.Background())
		if err != nil {
			glog.Errorf("Watch monitor failed: %s", err.Error())
			time.Sleep(cooldownTime)
			continue
		}
		glog.Flush()
		for event := range ch {
			glog.Flush()
			data := make(map[string][]string)
			if err = json.Unmarshal(event.Data, &data); err == nil {
				targetsLock.Lock()
				rp.targets = data[rp.serviceMode]
				rp.roundrobinIdx = -1
				targetsLock.Unlock()
				glog.V(1).Infof("Proxy %s successfully. Mode: %s, Port: %d, Targets: %v", event.Event, rp.serviceMode, rp.servicePort, rp.targets)
				glog.Flush()
			} else {
				glog.Errorf("Unmarshal monitor data error: %s", err.Error())
			}
			time.Sleep(cooldownTime)
		}
		glog.Flush()
		time.Sleep(cooldownTime)
	}

}

func (rp *MySQLProxy) listenConnectRequest() {
	for {
		// Listen the connection at servicePort
		listener, err := net.Listen("tcp", ":"+strconv.Itoa(rp.servicePort))
		if err != nil {
			glog.Error(err)
		} else {
			wg := &sync.WaitGroup{}
			for {
				glog.V(2).Info("Listen to other clients' request")
				targetsLock.RLock()
				targetsLen := len(rp.targets)
				targetsLock.RUnlock()
				if targetsLen == 0 {
					glog.V(1).Infof("Waiting for targets infomation. Recheck in %s", cooldownTime)
					time.Sleep(cooldownTime)
				} else if conn, err := listener.Accept(); err == nil {
					wg.Add(1)
					go func(conn net.Conn) {
						defer wg.Done()
						defer conn.Close()
						glog.V(2).Infof("Accepted: %s", conn.RemoteAddr())
						rp.handleRequest(conn)

					}(conn)
				} else {
					glog.Error(err)
					break
				}
			}
			wg.Wait()
		}
		if err := listener.Close(); err != nil {
			glog.Error(err)
		}
		time.Sleep(cooldownTime)
	}

}

func (rp *MySQLProxy) handleRequest(client net.Conn) {
	// Find an endpoint in RR algorithm
	targetsLock.RLock()
	if len(rp.targets) == 0 {
		glog.Errorf("No suitable targets")
		targetsLock.RUnlock()
		return
	}
	rp.roundrobinIdx++
	if rp.roundrobinIdx >= len(rp.targets) {
		rp.roundrobinIdx = 0
	}
	targetEndpoint := rp.targets[rp.roundrobinIdx]
	targetsLock.RUnlock()

	//得到目标地址后,建立proxy到目标地址的连接
	target, err := net.Dial("tcp", targetEndpoint)

	if err != nil {
		glog.Error(err)
		return
	}
	defer target.Close()
	pipe(client, target)

}

func pipe(serverConn, clientConn net.Conn) {
	isClientClosed := make(chan struct{}, 1)
	isServerClosed := make(chan struct{}, 1)

	go broker(serverConn, clientConn, isClientClosed)
	go broker(clientConn, serverConn, isServerClosed)

	var waitFor chan struct{}
	select {
	case <-isClientClosed:
		serverConn.Close()
		waitFor = isServerClosed
	case <-isServerClosed:
		clientConn.Close()
		waitFor = isClientClosed
	}
	<-waitFor
}

func broker(dst, src net.Conn, isSrcClosed chan<- struct{}) {
	srcAddr := src.RemoteAddr()
	dstAddr := dst.RemoteAddr()

	io.Copy(dst, src)
	isSrcClosed <- struct{}{}

	glog.V(1).Infof("%s --> %s is done", srcAddr, dstAddr)
}

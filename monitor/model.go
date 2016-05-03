package monitor

import (
	"fmt"

	"net"
	"net/http"
	"time"

	"github.com/ericpai/msops"
)

type PatchAction string
type GetType string

const (
	ActionActive          PatchAction = "active"
	ActionDetach          PatchAction = "detach"
	ActionPause           PatchAction = "pause"
	ActionRegisterMaster  PatchAction = "master"
	ActionRegisterStandby PatchAction = "standby"
	ActionRegisterSlave   PatchAction = "slave"
	ActionResume          PatchAction = "resume"
	ActionSwtich          PatchAction = "switch"
	ActionUnregister      PatchAction = "unregister"

	GetAllOverview GetType = "overview"
	GetOneDetails  GetType = "detail"
)

type InstanceModel struct {
	Role              string
	Addr              string
	Port              string
	InstanceStatus    msops.InstanceStatus
	ReplicationStatus msops.ReplicationStatus
}

func getInstance(endpoint string) (InstanceModel, int, error) {

	result := InstanceModel{}
	instSt := msops.CheckInstance(endpoint)
	var err error
	if _, exist := msMonitor.unregistered[endpoint]; !exist && instSt == msops.InstanceUnregistered {
		return result, http.StatusNotFound, fmt.Errorf("%s is not registered", endpoint)
	}
	if result.Addr, result.Port, err = net.SplitHostPort(endpoint); err != nil {
		return result, http.StatusBadRequest, err
	}
	result.InstanceStatus = instSt
	if endpoint == msMonitor.master {
		result.Role = "Master"
		if msMonitor.standby != "" {
			result.ReplicationStatus = msops.CheckReplication(endpoint, msMonitor.standby)
		} else {
			result.ReplicationStatus = msops.ReplicationNone
		}
	} else {
		if endpoint == msMonitor.standby {
			result.Role = "Standby"
			result.ReplicationStatus = msops.CheckReplication(endpoint, msMonitor.master)
		} else if instSt == msops.InstanceUnregistered {
			result.Role = "Unregistered"
			result.ReplicationStatus = msops.ReplicationNone
		} else {
			result.Role = "Slave"
			result.ReplicationStatus = msops.CheckReplication(endpoint, msMonitor.master)
		}
	}
	return result, http.StatusOK, nil
}

func getAllInstances() ([]InstanceModel, int, error) {
	result := make([]InstanceModel, 0, len(msMonitor.unregistered)+len(msMonitor.slave)+2)
	if msMonitor.master != "" {
		if inst, code, err := getInstance(msMonitor.master); err == nil {
			result = append(result, inst)
		} else {
			return result, code, err
		}
	}
	if msMonitor.standby != "" {
		if inst, code, err := getInstance(msMonitor.standby); err == nil {
			result = append(result, inst)
		} else {
			return result, code, err
		}
	}
	for endpoint := range msMonitor.slave {
		if inst, code, err := getInstance(endpoint); err == nil {
			result = append(result, inst)
		} else {
			return result, code, err
		}
	}
	for endpoint := range msMonitor.unregistered {
		if inst, code, err := getInstance(endpoint); err == nil {
			result = append(result, inst)
		} else {
			return result, code, err
		}
	}
	return result, http.StatusOK, nil
}

func active(endpoint string) (int, error) {
	var master = msMonitor.master
	if endpoint == msMonitor.master {
		master = msMonitor.standby
	}
	if st := msops.CheckReplication(endpoint, master); st != msops.ReplicationNone {
		return http.StatusForbidden, fmt.Errorf("The slave is not in detached mode")
	}

	if err := msops.ChangeMasterTo(endpoint, master, true); err != nil {
		return http.StatusInternalServerError, err
	}
	if err := msops.StartSlave(endpoint); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusAccepted, nil
}

func detach(endpoint string) (int, error) {
	if err := msops.StopSlave(endpoint); err != nil {
		return http.StatusInternalServerError, err
	}
	if err := msops.ResetSlave(endpoint, true); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusAccepted, nil
}

func pause(endpoint string) (int, error) {
	if endpoint == msMonitor.master {
		if err := msops.SetGlobalVariable(endpoint, "read_only", 1); err != nil {
			return http.StatusInternalServerError, err
		}
		return http.StatusAccepted, nil
	}
	if err := msops.StopSlave(endpoint); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusAccepted, nil
}

func register(endpoint string, role PatchAction) (int, error) {
	if _, exist := msMonitor.unregistered[endpoint]; !exist {
		return http.StatusForbidden, fmt.Errorf("%s is registered", endpoint)
	}
	newConf := getSecretConf()
	switch role {
	case ActionRegisterMaster:
		if msMonitor.master != "" {
			return http.StatusForbidden, fmt.Errorf("Master is registered")
		}
		if err := msops.Register(endpoint, dbaUser, newConf["dba_passwd"], replUser, newConf["repl_passwd"], connParam); err != nil {
			return http.StatusInternalServerError, err
		}
		msMonitor.master = endpoint
	case ActionRegisterSlave:
		if msMonitor.master == "" {
			return http.StatusForbidden, fmt.Errorf("Master is not registered")
		}
		if err := msops.Register(endpoint, dbaUser, newConf["dba_passwd"], replUser, newConf["repl_passwd"], connParam); err != nil {
			return http.StatusInternalServerError, err
		}
		msMonitor.slave[endpoint] = placeHolder
	case ActionRegisterStandby:
		if msMonitor.master == "" {
			return http.StatusForbidden, fmt.Errorf("Master is not registered")
		}
		if msMonitor.standby != "" {
			return http.StatusForbidden, fmt.Errorf("Standby is registered")
		}
		if err := msops.Register(endpoint, dbaUser, newConf["dba_passwd"], replUser, newConf["repl_passwd"], connParam); err != nil {
			return http.StatusInternalServerError, err
		}
		msMonitor.standby = endpoint
	}
	delete(msMonitor.unregistered, endpoint)
	return http.StatusAccepted, nil
}

func resume(endpoint string) (int, error) {
	if endpoint == msMonitor.master {
		if err := msops.SetGlobalVariable(endpoint, "read_only", 0); err != nil {
			return http.StatusInternalServerError, err
		}
		return http.StatusAccepted, nil
	}
	if err := msops.StartSlave(endpoint); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusAccepted, nil
}

func switchToMaster(endpoint string) (int, error) {
	if endpoint == msMonitor.master {
		return http.StatusForbidden, fmt.Errorf("%s is already master now", endpoint)
	}
	if msMonitor.master == "" {
		return http.StatusForbidden, fmt.Errorf("Master is not registered")
	}
	if _, exist := msMonitor.unregistered[endpoint]; exist {
		return http.StatusForbidden, fmt.Errorf("%s is not registered", endpoint)
	}

	if err := msops.KillProcesses(msMonitor.master, sysUsers...); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("Pre-killing failed: %s", err.Error())
	}
	if err := msops.SetGlobalVariable(msMonitor.master, "read_only", 1); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("Enable read_only failed: %s", err.Error())
	}
	if err := msops.KillProcesses(msMonitor.master, sysUsers...); err != nil {
		msops.SetGlobalVariable(msMonitor.master, "read_only", 0)
		return http.StatusInternalServerError, fmt.Errorf("Post-killing failed: %s", err.Error())
	}
	if st := msops.CheckReplication(endpoint, msMonitor.master); st != msops.ReplicationOK {
		time.Sleep(3 * time.Second)
		if st = msops.CheckReplication(endpoint, msMonitor.master); st != msops.ReplicationOK {
			msops.SetGlobalVariable(msMonitor.master, "read_only", 0)
			return http.StatusInternalServerError, fmt.Errorf("Replication status of %s is not OK", endpoint)
		}
	}
	rev := msops.CheckReplication(msMonitor.master, msMonitor.standby) == msops.ReplicationOK
	if err := msops.StopSlave(msMonitor.master); err != nil {
		msops.SetGlobalVariable(msMonitor.master, "read_only", 0)
		return http.StatusInternalServerError, fmt.Errorf("Stop slave failed: %s", err.Error())
	}
	if err := msops.ChangeMasterTo(msMonitor.master, endpoint, true); err != nil {
		msops.StartSlave(msMonitor.master)
		msops.SetGlobalVariable(msMonitor.master, "read_only", 0)
		return http.StatusInternalServerError, fmt.Errorf("Change master failed: %s", err.Error())
	}
	if endpoint != msMonitor.standby {
		msMonitor.slave[msMonitor.master] = placeHolder
		delete(msMonitor.slave, endpoint)
	} else {
		msMonitor.standby = msMonitor.master
	}
	msMonitor.master = endpoint

	msops.StopSlave(msMonitor.master)
	msops.ResetSlave(msMonitor.master, true)

	// Now switch successfully
	if msMonitor.standby != "" {
		msops.StopSlave(msMonitor.standby)
		msops.ChangeMasterTo(msMonitor.standby, msMonitor.master, true)
		msops.StartSlave(msMonitor.standby)

		if rev {
			msops.ChangeMasterTo(msMonitor.master, msMonitor.standby, true)
			msops.StartSlave(msMonitor.master)
		}
	}
	for slaveEndpoint := range msMonitor.slave {
		msops.StopSlave(slaveEndpoint)
		msops.ChangeMasterTo(slaveEndpoint, msMonitor.master, true)
		msops.StartSlave(slaveEndpoint)
	}
	msops.SetGlobalVariable(msMonitor.master, "read_only", 0)
	return http.StatusAccepted, nil
}

func unregister(endpoint string) (int, error) {
	if endpoint == msMonitor.master {
		return http.StatusForbidden, fmt.Errorf("Master is not allowed to be unregistered")
	}
	if endpoint == msMonitor.standby {
		msMonitor.standby = ""
	} else {
		delete(msMonitor.slave, endpoint)
	}
	msMonitor.unregistered[endpoint] = placeHolder
	msops.Unregister(endpoint)
	return http.StatusAccepted, nil
}

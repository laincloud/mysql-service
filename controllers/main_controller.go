package controllers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/astaxie/beego"
	"github.com/laincloud/mysql-service/monitor"
)

type MainController struct {
	beego.Controller
}

func (c *MainController) Overview() {
	c.Data["prevAddr"] = "#"
	c.Data["menu"] = "overview"
	getReq := monitor.GetRequest{
		RequestType:  monitor.GetAllOverview,
		ResponseChan: make(chan monitor.GetResponse),
	}
	monitor.Get(getReq)
	resp := <-getReq.ResponseChan
	if resp.Err != nil {
		c.handleError("Get overview error", resp.Err.Error(), resp.Code)
	} else {
		var insts []monitor.InstanceView
		json.Unmarshal(resp.Data, &insts)
		c.Data["Instances"] = insts
		c.Layout = "frame.html"
		c.TplNames = "overview.html"
	}
}

func (c *MainController) Details() {
	endpoint := net.JoinHostPort(c.GetString("host"), c.GetString("port"))
	c.Data["prevAddr"] = endpoint
	c.Data["menu"] = "details"
	getReq := monitor.GetRequest{
		RequestType:  monitor.GetOneDetails,
		Params:       map[string]string{"endpoint": endpoint},
		ResponseChan: make(chan monitor.GetResponse),
	}
	monitor.Get(getReq)
	singleResp := <-getReq.ResponseChan
	getReq.RequestType = monitor.GetAllOverview
	monitor.Get(getReq)
	allResp := <-getReq.ResponseChan
	if singleResp.Err != nil {
		c.handleError(fmt.Sprintf("Get details of %s error", endpoint), singleResp.Err.Error(), singleResp.Code)
	} else if allResp.Err != nil {
		c.handleError("Get servers list error", allResp.Err.Error(), allResp.Code)
	} else {
		var inst monitor.InstanceView
		var insts []monitor.InstanceView
		json.Unmarshal(singleResp.Data, &inst)
		json.Unmarshal(allResp.Data, &insts)
		c.Data["Instance"] = inst
		c.Data["Instances"] = insts
		c.TplNames = "details.html"
		c.Layout = "frame.html"
	}
}

func (c *MainController) Action() {
	endpoint := net.JoinHostPort(c.GetString("host"), c.GetString("port"))
	actionType := c.GetString("type")

	patchReq := monitor.PatchRequest{
		Action:       monitor.PatchAction(actionType),
		Endpoint:     endpoint,
		ResponseChan: make(chan monitor.PatchResponse),
	}
	monitor.Patch(patchReq)
	patchResp := <-patchReq.ResponseChan
	if patchResp.Err != nil {
		c.handleError(fmt.Sprintf("%s on %s error", actionType, endpoint), patchResp.Err.Error(), patchResp.Code)
	} else {
		c.Redirect("/", http.StatusFound)
	}
}

func (c *MainController) Error() {
	var (
		errNo int
		err   error
	)
	if errNo, err = c.GetInt("errNo"); err != nil {
		errNo = http.StatusForbidden
	}
	c.handleError(c.GetString("errTitle"), c.GetString("errMsg"), errNo)
}

func (c *MainController) handleError(errTitle, errMsg string, errNo int) {
	c.TplNames = "error.html"
	c.Data["errNo"] = errNo
	c.Data["errMsg"] = errMsg
	c.Data["errTitle"] = errTitle
}

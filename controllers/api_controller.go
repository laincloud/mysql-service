package controllers

import (
	"encoding/json"
	"fmt"

	"github.com/astaxie/beego"
	"github.com/laincloud/mysql-service/monitor"
)

type APIController struct {
	beego.Controller
}

func (c *APIController) GetRole() {
	addr := fmt.Sprintf("%s:%s", c.GetString("host"), c.GetString("port"))
	req := monitor.GetRequest{
		RequestType:  monitor.GetOneDetails,
		Params:       map[string]string{"endpoint": addr},
		ResponseChan: make(chan monitor.GetResponse),
	}
	monitor.Get(req)
	resp := <-req.ResponseChan
	var inst monitor.InstanceView
	if json.Unmarshal(resp.Data, &inst) != nil {
		inst.Role = "Unknown"
	}
	c.Ctx.WriteString(inst.Role)
}

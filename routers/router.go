package routers

import (
	"github.com/astaxie/beego"
	"github.com/laincloud/mysql-service/controllers"
)

func init() {
	beego.SetStaticPath("/etc/js", "etc/js")
	beego.SetStaticPath("/etc/css", "etc/css")
	beego.SetStaticPath("/etc/img", "etc/img")
	beego.SetStaticPath("/etc/fonts", "etc/fonts")
	beego.SetStaticPath("/etc/bower_components", "etc/bower_components")

	mainCtl := &controllers.MainController{}
	apiCtl := &controllers.APIController{}

	beego.Router("/", mainCtl, "get:Overview")
	beego.Router("/error", mainCtl, "get:Error")
	beego.Router("/details", mainCtl, "get:Details")
	beego.Router("/action", mainCtl, "get:Action")

	beego.Router("/role", apiCtl, "get:GetRole")

	beego.InsertFilter("/", beego.BeforeRouter, controllers.FilterConsoleLogin)
	beego.InsertFilter("/error", beego.BeforeRouter, controllers.FilterConsoleLogin)
	beego.InsertFilter("/action", beego.BeforeRouter, controllers.FilterConsoleLogin)
	beego.InsertFilter("/details", beego.BeforeRouter, controllers.FilterConsoleLogin)
}

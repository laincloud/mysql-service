package main

import (
	"flag"

	"github.com/astaxie/beego"
	_ "github.com/go-sql-driver/mysql"
	"github.com/laincloud/mysql-service/monitor"
	_ "github.com/laincloud/mysql-service/routers"
)

func main() {
	flag.Parse()
	go monitor.Start()

	beego.Run()
}

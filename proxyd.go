package main

import (
	"flag"

	"github.com/laincloud/mysql-service/proxy"
)

func main() {
	var servicePort int
	var serviceMode string
	flag.IntVar(&servicePort, "p", 3306, "The service port for mysql clients")
	flag.StringVar(&serviceMode, "m", "slave", "The service mode for mysql clients (master|slave)")
	flag.Parse()
	proxy.StartProxy(servicePort, serviceMode)
}

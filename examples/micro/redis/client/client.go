package main

import (
	"net"
	"time"

	"github.com/lesismal/arpc/extension/micro/redis"

	"github.com/lesismal/arpc/extension/micro"
	"github.com/lesismal/arpc/log"
)

func dialer(addr string) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, time.Second*3)
}

func main() {
	var (
		appPrefix = "app"
		service   = "echo"

		serviceManager = micro.NewServiceManager(dialer)
	)
	discovery, err := redis.NewDiscovery("localhost:6379", appPrefix, time.Second*5, serviceManager)
	if err != nil {
		log.Error("NewDiscovery failed: %v", err)
		panic(err)
	}
	defer discovery.Stop()

	for {
		client, err := serviceManager.ClientBy(service)
		if err != nil {
			log.Error("get Client failed: %v", err)
		} else {
			req := "hello"
			rsp := ""
			err = client.Call("/echo", &req, &rsp, time.Second*5)
			if err != nil {
				log.Info("Call /echo failed: %v", err)
			} else {
				log.Info("Call /echo Response: \"%v\"", rsp)
			}
		}
		time.Sleep(time.Second)
	}
}

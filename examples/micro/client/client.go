package main

import (
	"net"
	"time"

	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/micro"
	"github.com/lesismal/arpc/micro/etcd"
)

func dialer(addr string) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, time.Second*3)
}

func main() {
	var (
		appPrefix = "app"
		service   = "echo"

		endpoints = []string{"localhost:2379", "localhost:22379", "localhost:32379"}

		serviceManager = micro.NewServiceManager(dialer)
	)
	discovery, err := etcd.NewDiscovery(endpoints, appPrefix, serviceManager)
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

package main

import (
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	req := "hello"
	rsp := ""
	err = client.Call("/echo", &req, &rsp, time.Second*5)
	if err != nil {
		log.Error("Call /echo failed: %v", err)
		return
	}
	log.Info("Call /echo Response: \"%v\"", rsp)
}

package main

import (
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
)

func main() {
	clientNum := 10
	pool, err := arpc.NewClientPool(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
	}, clientNum)
	if err != nil {
		panic(err)
	}

	for i := 0; i < clientNum; i++ {
		req := "hello"
		rsp := ""
		err = pool.Next().Call("/echo", &req, &rsp, time.Second*5)
		if err != nil {
			log.Error("Call /echo failed: %v", err)
			return
		}
		log.Info("Call /echo Response: \"%v\"", rsp)
	}

	pool.Stop()

	time.Sleep(time.Second / 10)
}

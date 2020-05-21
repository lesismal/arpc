package main

import (
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
)

const (
	addr = "localhost:8888"

	method = "Hello"
)

// HelloReq .
type HelloReq struct {
	Msg string
}

// HelloRsp .
type HelloRsp struct {
	Msg string
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", addr, time.Second*3)
}

func main() {
	func() {
		client, err := arpc.NewClient(dialer)
		if err != nil {
			log.Println("NewClient failed:", err)
			return
		}

		client.Run()
		defer client.Stop()

		req := &HelloReq{Msg: "hello from client.Call"}
		rsp := &HelloRsp{}
		err = client.Call(method, req, rsp, time.Second*5)
		if err != nil {
			log.Printf("Call failed: %v", err)
		} else {
			log.Printf("Call Response: \"%v\"", rsp.Msg)
		}
	}()

	<-make(chan int)
}

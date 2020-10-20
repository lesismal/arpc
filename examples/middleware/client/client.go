package main

import (
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
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
	err = client.Call("/panic", &req, &rsp, time.Second*5)
	if err != nil {
		log.Fatalf("Call /panic failed: %v", err)
	} else {
		log.Printf("Call /panic Response: \"%v\"", rsp)
	}

	err = client.Call("/logger", &req, &rsp, time.Second*5)
	if err != nil {
		log.Fatalf("/logger: %v", err)
	} else {
		log.Printf("Call /logger Response: \"%v\"", rsp)
	}

}

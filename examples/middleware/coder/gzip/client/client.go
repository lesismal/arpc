package main

import (
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/middleware/coder/gzip"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	client.Handler.UseCoder(gzip.New())

	req := ""
	for i := 0; i < 2048; i++ {
		req += "a"
	}
	rsp := ""
	err = client.Call("/echo", &req, &rsp, time.Second*5)
	if err != nil {
		log.Fatalf("Call /echo failed: %v", err)
	} else {
		log.Printf("Call /echo Response: \"%v\"", rsp)
	}
}

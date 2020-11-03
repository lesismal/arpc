package main

import (
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/middleware/coder"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	client.Handler.UseCoder(coder.NewTracer("trade_app_test", uint64(time.Now().UnixNano())))

	for i := 0; i < 10; i++ {
		req := "hello"
		rsp := ""
		err = client.Call("/echo", &req, &rsp, time.Second*5)
		if err != nil {
			log.Fatalf("Call /echo failed: %v", err)
		} else {
			log.Printf("Call /echo Response: \"%v\"", rsp)
		}
	}
}

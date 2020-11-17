package main

import (
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/middleware/coder/tracer"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	tracer := tracer.New("appTrace", "span")
	client.Handler.UseCoder(tracer)

	sp := tracer.NewSpan()
	req := "hello"
	rsp := ""
	err = client.Call("/step_1", &req, &rsp, time.Second*5, sp.Values())
	if err != nil {
		log.Fatalf("Call /step_1 failed: %v", err)
	} else {
		log.Printf("Call /step_1 Response: \"%v\"", rsp)
	}
}

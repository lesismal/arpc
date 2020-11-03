package main

import (
	"fmt"
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

	client.Handler.UseCoder(coder.NewTracer("trace_app_test", "span_test", uint64(time.Now().UnixNano())))

	for i := 0; i < 5; i++ {
		req := "hello"
		rsp := ""
		err = client.Call("/echo", &req, &rsp, time.Second*5)
		if err != nil {
			log.Fatalf("Call /echo failed: %v", err)
		} else {
			log.Printf("Call /echo Response: \"%v\"", rsp)
		}
	}

	for i := 0; i < 5; i++ {
		req := "hello"
		rsp := ""
		// err = client.Call("/echo", &req, &rsp, time.Second*5, map[string]interface{}{coder.TraceIdKey: "call_with_traceid", coder.SpanIdKey: fmt.Sprintf("call_with_spanid_%v", i)})
		err = client.Call("/echo", &req, &rsp, time.Second*5, arpc.M{coder.TraceIdKey: "call_with_traceid", coder.SpanIdKey: fmt.Sprintf("call_with_spanid_%v", i)})
		if err != nil {
			log.Fatalf("Call /echo failed: %v", err)
		} else {
			log.Printf("Call /echo Response: \"%v\"", rsp)
		}
	}
}

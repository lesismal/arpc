package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/coder/msgpack"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	client.Handler.UseCoder(msgpack.New())

	for i := 0; i < 5; i++ {
		req := fmt.Sprintf("hello %v", i)
		rsp := ""
		midlewareValues := map[interface{}]interface{}{}
		k, v := fmt.Sprintf("key-%v", i), fmt.Sprintf("value-%v", i)
		midlewareValues[k] = v
		err = client.Call("/echo", &req, &rsp, time.Second*5, midlewareValues)
		if err != nil {
			log.Fatalf("Call /echo failed: %v", err)
		} else {
			log.Printf("Call /echo Response: \"%v\"", rsp)
		}
	}
}

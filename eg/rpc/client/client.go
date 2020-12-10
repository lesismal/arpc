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
	err = client.Call("/echo/sync", &req, &rsp, time.Second*5)
	if err != nil {
		log.Fatalf("Call /echo/sync failed: %v", err)
	} else {
		log.Printf("Call /echo/sync Response: \"%v\"", rsp)
	}
	err = client.Call("/echo/async", &req, &rsp, time.Second*5)
	if err != nil {
		log.Fatalf("Call /echo/async failed: %v", err)
	} else {
		log.Printf("Call /echo/async Response: \"%v\"", rsp)
	}
	done := make(chan string)
	err = client.CallAsync("/echo/async", &req, func(ctx *arpc.Context) {
		rsp := ""
		err = ctx.Bind(&rsp)
		if err != nil {
			log.Fatalf("Call /echo/async Bind failed: %v", err)
		}
		if rsp != req {
			log.Fatalf("Call /echo/async failed: %v", err)
		}
		done <- rsp
	}, time.Second*5)
	if err != nil {
		log.Fatalf("Call /echo/async failed: %v", err)
	} else {
		rsp := <-done
		log.Printf("CallAsync /echo/async Response: \"%v\"", rsp)
	}
}

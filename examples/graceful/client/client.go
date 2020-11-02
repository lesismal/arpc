package main

import (
	"log"
	"net"
	"sync"
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

	wg := sync.WaitGroup{}
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := "hello"
			rsp := ""
			err = client.Call("/echo", &req, &rsp, time.Second*10)
			if err != nil {
				log.Fatalf("Call /echo failed: %v", err)
			} else {
				log.Printf("Call /echo Response: \"%v\"", rsp)
			}
		}()
	}
	wg.Wait()

	req := "hello"
	rsp := ""
	err = client.Call("/echo", &req, &rsp, time.Second*10)
	if err != nil {
		log.Fatalf("Call /echo failed: %v", err)
	} else {
		log.Printf("Call /echo Response: \"%v\"", rsp)
	}
}

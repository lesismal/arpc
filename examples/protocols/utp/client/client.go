package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/anacrolix/utp"
	"github.com/lesismal/arpc"
)

func main() {
	client, err := arpc.NewClient(func() (net.Conn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return utp.DialContext(ctx, "localhost:8888")
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	req := "hello"
	rsp := ""
	err = client.Call("/echo", &req, &rsp, time.Second*5)
	if err != nil {
		log.Fatalf("Call failed: %v", err)
	} else {
		log.Printf("Call Response: \"%v\"", rsp)
	}
}

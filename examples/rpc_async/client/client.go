package main

import (
	"log"
	"net"
	"os"
	"time"

	"github.com/lesismal/arpc"
)

// OnClientCallAsyncResponse .
func OnClientCallAsyncResponse(ctx *arpc.Context) {
	ret := ""
	err := ctx.Bind(&ret)
	log.Printf("OnClientCallAsyncResponse: \"%v\", error: %v", ret, err)
	os.Exit(0)
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
}

func main() {
	client, err := arpc.NewClient(dialer)
	if err != nil {
		log.Println("NewClient failed:", err)
		return
	}

	client.Run()
	payload := "hello from client.CallAsync"
	client.CallAsync("/echo", payload, OnClientCallAsyncResponse, time.Second)
	defer client.Stop()

	<-make(chan int)
}

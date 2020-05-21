package main

import (
	"log"
	"net"
	"os"
	"time"

	"github.com/lesismal/arpc"
)

const (
	addr = "localhost:8888"

	method = "Hello"
)

func OnClientCallAsyncResponse(ctx *arpc.Context) {
	ret := ""
	ctx.Bind(&ret)
	log.Printf("OnClientCallAsyncResponse: \"%v\"", ret)
	os.Exit(0)
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", addr, time.Second*3)
}

func main() {
	client, err := arpc.NewClient(dialer)
	if err != nil {
		log.Println("NewClient failed:", err)
		return
	}

	client.Run()
	payload := "hello from client.CallAsync"
	client.CallAsync(method, payload, OnClientCallAsyncResponse, time.Second)
	defer client.Stop()

	<-make(chan int)
}

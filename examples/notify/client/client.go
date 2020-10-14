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

	methodHello  = "Hello"
	methodNotify = "Notify"
)

var notifyCount = 3

// OnServerNotify .
func OnServerNotify(ctx *arpc.Context) {
	ret := ""
	ctx.Bind(&ret)
	log.Printf("OnServerNotify: \"%v\"", ret)
	notifyCount--
	if notifyCount == 0 {
		os.Exit(0)
	}
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

	// register handler for method
	client.Handler.Handle(methodNotify, OnServerNotify)
	defer client.Stop()

	payload := "hello from client.Call"
	response := ""
	err = client.Call(methodHello, payload, &response, time.Second*5)
	if err != nil {
		log.Printf("Call failed: %v", err)
	} else {
		log.Printf("Call Response: \"%v\"", response)
	}

	<-make(chan int)
}

package main

import (
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
)

const (
	addr = ":8888"

	methodHello  = "Hello"
	methodNotify = "Notify"
)

// OnClientHello .
func OnClientHello(ctx *arpc.Context) {
	str := ""
	ctx.Bind(&str)

	log.Printf("OnClientHello: \"%v\"", str)

	// async response should Clone a Context to Write
	go ctx.Clone().Write(str)

	// send 3 notify messages
	go func() {
		notifyPayload := "notify from server, nonblock"
		ctx.Client.Notify(methodNotify, notifyPayload, arpc.TimeZero)

		notifyPayload = "notify from server, block"
		ctx.Client.Notify(methodNotify, notifyPayload, arpc.TimeForever)

		notifyPayload = "notify from server, with 1 second timeout"
		ctx.Client.Notify(methodNotify, notifyPayload, time.Second)
	}()
}

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := arpc.NewServer()
	svr.Handler.Handle(methodHello, OnClientHello)

	svr.Serve(ln)
}

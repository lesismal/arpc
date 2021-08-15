package main

import (
	"log"
	"sync"
	"time"

	"github.com/lesismal/arpc"
)

var mux = sync.RWMutex{}
var server = arpc.NewServer()
var clientMap = make(map[*arpc.Client]struct{})

func main() {
	server.Handler.EnablePool(true)

	server.Handler.Handle("/echo", func(ctx *arpc.Context) {
		var data []byte
		err := ctx.Bind(&data)
		if err != nil {
			log.Fatalf("invalid request: %v", err)
		}
		ctx.Write(data)
	})

	// add client
	server.Handler.HandleConnected(func(c *arpc.Client) {
		mux.Lock()
		clientMap[c] = struct{}{}
		mux.Unlock()
	})
	// remove client
	server.Handler.HandleDisconnected(func(c *arpc.Client) {
		mux.Lock()
		delete(clientMap, c)
		mux.Unlock()
	})

	go func() {
		ticker := time.NewTicker(time.Second / 100)
		for i := 0; true; i++ {
			<-ticker.C
			broadcast()
		}
	}()

	server.Run("localhost:8888")
}

func broadcast() {
	msg := server.NewMessage(arpc.CmdNotify, "/broadcast", "broadcast msg")
	mux.RLock()
	defer func() {
		mux.RUnlock()
		msg.Release()
	}()

	for client := range clientMap {
		msg.Retain()
		client.PushMsg(msg, arpc.TimeZero)
	}
}

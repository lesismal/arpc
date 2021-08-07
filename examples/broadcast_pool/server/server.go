package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/nbio/mempool"
)

var mux = sync.RWMutex{}
var server = arpc.NewServer()
var clientMap = make(map[*arpc.Client]struct{})

func initPool() {
	server.Handler.HandleMalloc(func(size int) []byte {
		return mempool.Malloc(size)
	})
	server.Handler.HandleFree(func(buf []byte) {
		mempool.Free(buf)
	})
	server.Handler.HandleContextDone(func(ctx *arpc.Context) {
		ctx.Release()
	})
	server.Handler.HandleMessageDone(func(c *arpc.Client, m *arpc.Message) {
		m.ReleaseAndPayback(c.Handler)
	})
}

func main() {
	initPool()

	server.Handler.Handle("/enter", func(ctx *arpc.Context) {
		passwd := ""
		ctx.Bind(&passwd)
		if passwd == "123qwe" {
			// keep client
			mux.Lock()
			clientMap[ctx.Client] = struct{}{}
			mux.Unlock()

			log.Printf("enter success")
		} else {
			log.Printf("enter failed invalid passwd: %v", passwd)
			ctx.Client.Stop()
		}
	})
	// release client
	server.Handler.HandleDisconnected(func(c *arpc.Client) {
		mux.Lock()
		delete(clientMap, c)
		mux.Unlock()
	})

	go func() {
		ticker := time.NewTicker(time.Second)
		for i := 0; true; i++ {
			<-ticker.C
			broadcast(i)
		}
	}()

	server.Run("localhost:8888")
}

func broadcast(i int) {
	msg := server.NewMessage(arpc.CmdNotify, "/broadcast", fmt.Sprintf("broadcast msg %d", i))
	mux.RLock()
	defer func() {
		mux.RUnlock()
		msg.ReleaseAndPayback(server.Handler)
	}()

	for client := range clientMap {
		msg.Retain()
		client.PushMsg(msg, arpc.TimeZero)
	}
}

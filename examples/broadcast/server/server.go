package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
)

var mux = sync.RWMutex{}
var clientMap = make(map[*arpc.Client]struct{})

func main() {
	server := arpc.NewServer()
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

	server.Run(":8888")
}

func broadcast(i int) {
	msg := arpc.NewMessage(arpc.CmdNotify, "/broadcast", fmt.Sprintf("broadcast msg %d", i), codec.DefaultCodec)
	mux.RLock()
	for client := range clientMap {
		client.PushMsg(msg, arpc.TimeZero)
	}
	mux.RUnlock()
}

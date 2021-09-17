package main

import (
	"fmt"
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

	server.Handler.Handle("/enter", func(ctx *arpc.Context) {
		passwd := ""
		ctx.Bind(&passwd)
		if passwd == "123qwe" {
			// keep client
			mux.Lock()
			clientMap[ctx.Client] = struct{}{}
			mux.Unlock()

			ctx.Write(nil)

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
			switch i % 4 {
			case 0:
				server.Broadcast("/broadcast", fmt.Sprintf("Broadcast msg %d", i))
			case 1:
				server.BroadcastWithFilter("/broadcast", fmt.Sprintf("BroadcastWithFilter msg %d", i), func(c *arpc.Client) bool {
					return true
				})
			case 2:
				server.ForEach(func(c *arpc.Client) {
					c.Notify("/broadcast", fmt.Sprintf("ForEach msg %d", i), arpc.TimeZero)
				})
			case 3:
				server.ForEachWithFilter(func(c *arpc.Client) {
					c.Notify("/broadcast", fmt.Sprintf("ForEachWithFilter msg %d", i), arpc.TimeZero)
				}, func(c *arpc.Client) bool {
					return true
				})

			}
		}
	}()

	server.Run("localhost:8888")
}

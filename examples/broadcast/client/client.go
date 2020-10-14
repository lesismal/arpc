package main

import (
	"log"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/lesismal/arpc"
)

var notifyCount int32

// OnBroadcast .
func OnBroadcast(ctx *arpc.Context) {
	ret := ""
	ctx.Bind(&ret)
	log.Printf("OnServerNotify: \"%v\"", ret)
	if atomic.AddInt32(&notifyCount, 1) >= 20 {
		os.Exit(0)
	}
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
}

func main() {
	var clients []*arpc.Client

	arpc.DefaultHandler.Handle("/broadcast", OnBroadcast)

	for i := 0; i < 10; i++ {
		client, err := arpc.NewClient(dialer)
		if err != nil {
			log.Println("NewClient failed:", err)
			return
		}
		defer client.Stop()

		clients = append(clients, client)
	}

	for i := 0; i < 10; i++ {
		client := clients[i]
		go func() {

			passwd := "123qwe"
			response := ""
			err := client.Call("/enter", passwd, &response, time.Second*5)
			if err != nil {
				log.Printf("Call failed: %v", err)
			} else {
				log.Printf("Call Response: \"%v\"", response)
			}
		}()
	}

	<-make(chan int)
}

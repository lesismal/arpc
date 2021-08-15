package main

import (
	"bytes"
	"crypto/rand"
	"flag"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/lesismal/arpc"
)

var qps int64
var qpsBroadcast int64
var clientNum = flag.Int("pool", 10, "client number")
var concurrency = flag.Int("c", 100, "concurrency number for each cleint")

// OnBroadcast .
func OnBroadcast(ctx *arpc.Context) {
	data := ""
	ctx.Bind(&data)
	if data != "broadcast msg" {
		log.Fatalf("dirty data: %v", data)
	}
	atomic.AddInt64(&qpsBroadcast, 1)
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", "localhost:8888", time.Second*3)
}

func main() {
	flag.Parse()

	arpc.EnablePool(true)

	var clients []*arpc.Client

	arpc.DefaultHandler.Handle("/broadcast", OnBroadcast)

	for i := 0; i < *clientNum; i++ {
		client, err := arpc.NewClient(dialer)
		if err != nil {
			log.Println("NewClient failed:", err)
			return
		}
		defer client.Stop()

		clients = append(clients, client)
	}

	for i := 0; i < *clientNum; i++ {
		client := clients[i]
		for j := 0; j < *concurrency; j++ {
			go func() {
				var (
					req = make([]byte, 1024)
					res []byte
				)
				for {
					rand.Read(req)
					err := client.Call("/echo", req, &res, time.Second*5)
					if err != nil {
						log.Fatalf("Call failed: %v", err)
					}
					if !bytes.Equal(req, res) {
						log.Fatalf("invalid response")
					}
					atomic.AddInt64(&qps, 1)
				}
			}()
		}
	}

	var total int64 = 0
	var totalBroadcast int64 = 0
	for {
		time.Sleep(time.Second)
		tmpQps := atomic.SwapInt64(&qps, 0)
		total += tmpQps
		tmpBroadcast := atomic.SwapInt64(&qpsBroadcast, 0)
		totalBroadcast += tmpBroadcast

		log.Printf("qps: %v, total: %v, broadcast: %v, broadcastTotal: %v", tmpQps, total, tmpBroadcast, totalBroadcast)
	}
}

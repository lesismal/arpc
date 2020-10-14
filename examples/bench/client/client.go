package main

import (
	"log"
	"net"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/lesismal/arpc"
)

var (
	addr = "localhost:8888"

	method = "Hello"
)

// HelloReq .
type HelloReq struct {
	Msg string
}

// HelloRsp .
type HelloRsp struct {
	Msg string
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", addr, time.Second*3)
}

func main() {
	var (
		qpsSec                 uint64
		qpsTotal               uint64
		clientNum              = runtime.NumCPU() * 2
		eachClientCoroutineNum = 10
	)

	clients := make([]*arpc.Client, clientNum)

	for i := 0; i < clientNum; i++ {
		client, err := arpc.NewClient(dialer)
		if err != nil {
			log.Println("NewClient failed:", err)
			return
		}
		clients[i] = client
		defer client.Stop()
	}

	for i := 0; i < clientNum; i++ {
		client := clients[i]
		for j := 0; j < eachClientCoroutineNum; j++ {
			go func() {
				var err error
				for k := 0; true; k++ {
					req := &HelloReq{Msg: "hello from client.Call"}
					rsp := &HelloRsp{}
					err = client.Call(method, req, rsp, time.Second*5)
					if err != nil {
						log.Printf("Call failed: %v", err)
					} else {
						//log.Printf("Call Response: \"%v\"", rsp.Msg)
						atomic.AddUint64(&qpsSec, 1)
					}
				}
			}()
		}
	}

	ticker := time.NewTicker(time.Second)
	for i := 0; true; i++ {
		if _, ok := <-ticker.C; !ok {
			return
		}
		if i < 3 {
			log.Printf("[qps preheating %v: %v]", i+1, atomic.SwapUint64(&qpsSec, 0))
			continue
		}
		qps := atomic.SwapUint64(&qpsSec, 0)
		qpsTotal += qps
		log.Printf("[qps: %v], [avg: %v / s], [total: %v, %v s]",
			qps, int64(float64(qpsTotal)/float64(i-2)), qpsTotal, int64(float64(i-2)))
	}
}

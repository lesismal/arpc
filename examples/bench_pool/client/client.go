package main

import (
	"fmt"
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
		asyncTimes             uint64
		qpsTotal               uint64
		failedTotal            uint64
		clientNum              = runtime.NumCPU() * 2
		eachClientCoroutineNum = 10
	)

	arpc.EnablePool(true)

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
		for j := 0; j < eachClientCoroutineNum-1; j++ {
			go func() {
				var err error
				for k := 0; true; k++ {
					req := &HelloReq{Msg: fmt.Sprintf("[%v] %v", client.Conn.LocalAddr(), k)}
					rsp := &HelloRsp{}
					err = client.Call(method, req, rsp, time.Second*5)
					if err != nil || rsp.Msg != req.Msg {
						atomic.AddUint64(&failedTotal, 1)
						log.Printf("Call failed: %v", err)
					} else {
						//log.Printf("Call Response: \"%v\"", rsp.Msg)
						atomic.AddUint64(&qpsSec, 1)
					}
				}
			}()
		}
		go func() {
			ticker := time.NewTicker(time.Second)
			for i := 0; true; i++ {
				select {
				case <-ticker.C:
					req := &HelloReq{Msg: fmt.Sprintf("[%v] %v", client.Conn.LocalAddr(), i)}
					rsp := &HelloRsp{}
					err := client.CallAsync(method, req, func(ctx *arpc.Context) {
						err := ctx.Bind(rsp)
						if err != nil || rsp.Msg != req.Msg {
							log.Printf("CallAsync failed: %v", err)
							atomic.AddUint64(&failedTotal, 1)
						} else {
							//log.Printf("Call Response: \"%v\"", rsp.Msg)
							atomic.AddUint64(&qpsSec, 1)
							atomic.AddUint64(&asyncTimes, 1)
						}
					}, time.Second*5)
					if err != nil {
						log.Printf("CallAsync failed: %v", err)
						atomic.AddUint64(&failedTotal, 1)
					} else {
						//log.Printf("Call Response: \"%v\"", rsp.Msg)
						atomic.AddUint64(&qpsSec, 1)
						atomic.AddUint64(&asyncTimes, 1)
					}
				}
			}
		}()
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
		log.Printf("[qps: %v], [asyncTimes: %v], [avg: %v / s], [total failed: %v, success: %v, %v s]",
			qps, atomic.LoadUint64(&asyncTimes), int64(float64(qpsTotal)/float64(i-2)), atomic.LoadUint64(&failedTotal), qpsTotal, int64(float64(i-2)))
	}
}

package main

import (
	"net"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/middleware/coder/tracer"
)

func main() {
	go func() {
		serverTracer2 := tracer.New("appTrace", "span")
		svr2 := arpc.NewServer()
		svr2.Handler.UseCoder(serverTracer2)
		svr2.Handler.Handle("/step_2", func(ctx *arpc.Context) {
			req := ""
			rsp := ""
			ctx.Bind(&req)
			rsp = req + "_server2"
			ctx.Write(rsp)
			sp := tracer.Span(ctx.Values())
			log.Info("/step_2, traceid: %v, spanid: %v", sp.TraceID(), sp.SpanID())
		})

		svr2.Run(":9999")
	}()

	time.Sleep(time.Second / 10)
	clientTracer := tracer.New("appTrace", "span")
	client2, err := arpc.NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:9999", time.Second*3)
	})
	if err != nil {
		panic(err)
	}
	defer client2.Stop()
	client2.Handler.UseCoder(clientTracer)

	serverTracer1 := tracer.New("appTrace", "span")
	svr1 := arpc.NewServer()
	svr1.Handler.UseCoder(serverTracer1)
	svr1.Handler.Handle("/step_1", func(ctx *arpc.Context) {
		sp := tracer.Span(ctx.Values())
		log.Info("/step_1, traceid: %v, spanid: %v", sp.TraceID(), sp.SpanID())

		nextSpan := serverTracer1.NextSpan(ctx.Values())
		req := ""
		rsp := ""
		ctx.Bind(&req)
		req += "_from_server1"
		err = client2.Call("/step_2", &req, &rsp, time.Second*5, nextSpan.Values())
		if err == nil {
			ctx.Write(rsp)
		} else {
			rsp = req + "_server2Failed"
			ctx.Write(rsp)
		}
	})

	svr1.Run(":8888")
}

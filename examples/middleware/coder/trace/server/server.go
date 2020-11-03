package main

import (
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/middleware/coder"
)

func main() {
	svr := arpc.NewServer()

	svr.Handler.UseCoder(coder.NewTracer("", "", 0))

	// register router
	svr.Handler.Handle("/echo", func(ctx *arpc.Context) {
		ctx.Write(ctx.Body())
		traceId, _ := ctx.Get(coder.TraceIdKey)
		spanId, _ := ctx.Get(coder.SpanIdKey)
		log.Info("/echo handler, traceid: %v, spanid: %v", traceId, spanId)
	})

	svr.Run(":8888")
}

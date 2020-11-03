package main

import (
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/middleware/coder"
)

func main() {
	svr := arpc.NewServer()

	svr.Handler.UseCoder(coder.NewGzip())

	// register router
	svr.Handler.Handle("/echo", func(ctx *arpc.Context) {
		ctx.Write(ctx.Body())
		log.Info("/echo")
	})

	svr.Run(":8888")
}

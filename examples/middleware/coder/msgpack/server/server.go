package main

import (
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/coder/msgpack"
	"github.com/lesismal/arpc/log"
)

func main() {
	svr := arpc.NewServer()

	svr.Handler.UseCoder(msgpack.New())

	// register router
	svr.Handler.Handle("/echo", func(ctx *arpc.Context) {
		ctx.Write(ctx.Body())
		log.Info("/echo, %v", ctx.Values())
	})

	svr.Run("localhost:8888")
}

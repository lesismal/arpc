package main

import (
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/router"
	"github.com/lesismal/arpc/internal/log"
)

func main() {
	svr := arpc.NewServer()

	svr.Handler.Use(router.Recover())
	svr.Handler.Use(router.Logger())

	// register router
	svr.Handler.Handle("/panic", func(ctx *arpc.Context) {
		ctx.Write(ctx.Body())
		log.Info("/panic handler")
		panic(string(ctx.Body()))
	})

	// register router
	svr.Handler.Handle("/logger", func(ctx *arpc.Context) {
		ctx.Write(ctx.Body())
		log.Info("/logger handler")
		time.Sleep(time.Millisecond)
	})

	svr.Run(":8888")
}

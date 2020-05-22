package main

import "github.com/lesismal/arpc"

func main() {
	svr := arpc.NewServer()

	// register router
	svr.Handler.Handle("/echo", func(ctx *arpc.Context) {
		str := ""
		ctx.Bind(&str)
		ctx.Write(str)
	})

	svr.Run(":8888")
}

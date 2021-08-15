package main

import (
	"log"
	"net"

	"github.com/lesismal/arpc"
)

const (
	addr = "localhost:8888"
)

// HelloReq .
type HelloReq struct {
	Msg string
}

// HelloRsp .
type HelloRsp struct {
	Msg string
}

// OnHello .
func OnHello(ctx *arpc.Context) {
	req := &HelloReq{}
	ctx.Bind(req)
	ctx.Write(&HelloRsp{Msg: req.Msg})
}

func main() {
	arpc.EnablePool(true)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := arpc.NewServer()

	svr.Handler.Handle("Hello", OnHello)
	svr.Serve(ln)
}

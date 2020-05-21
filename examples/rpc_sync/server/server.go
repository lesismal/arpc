package main

import (
	"log"
	"net"

	"github.com/lesismal/arpc"
)

const (
	addr = ":8888"

	method = "Hello"
)

type HelloReq struct {
	Msg string
}

type HelloRsp struct {
	Msg string
}

func OnHello(ctx *arpc.Context) {
	req := &HelloReq{}
	rsp := &HelloRsp{}

	ctx.Bind(req)
	log.Printf("OnHello: \"%v\"", req.Msg)

	rsp.Msg = req.Msg
	ctx.Write(rsp)
}

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := arpc.NewServer()
	svr.Handler.Handle("Hello", OnHello)
	svr.Serve(ln)
}

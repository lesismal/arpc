package main

import (
	"log"
	"net"

	"github.com/lesismal/arpc"
	"github.com/lesismal/nbio/mempool"
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

func initPool(svr *arpc.Server) {
	svr.Handler.HandleMalloc(func(size int) []byte {
		return mempool.Malloc(size)
	})
	svr.Handler.HandleFree(func(buf []byte) {
		mempool.Free(buf)
	})
	svr.Handler.HandleContextDone(func(ctx *arpc.Context) {
		ctx.Release()
	})
	svr.Handler.HandleMessageDone(func(c *arpc.Client, m *arpc.Message) {
		m.ReleaseAndPayback(c.Handler)
	})
}

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := arpc.NewServer()

	initPool(svr)

	svr.Handler.Handle("Hello", OnHello)
	svr.Serve(ln)
}

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

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := arpc.NewServer()

	bufferPool := mempool.New(1024 * 1024 * 16)
	svr.Handler.HandleMalloc(func(i int) []byte {
		// return mempool.Malloc(i)
		return bufferPool.Malloc(i)
	})
	svr.Handler.HandleFree(func(buf []byte) {
		// mempool.Free(buf)
		bufferPool.Free(buf)
	})
	svr.Handler.HandleContextDone(func(ctx *arpc.Context) {
		ctx.Release()
	})
	svr.Handler.HandleMessageDone(func(c *arpc.Client, m *arpc.Message) {
		m.ReleaseAndPayback(c.Handler)
	})

	svr.Handler.Handle("Hello", OnHello)
	svr.Serve(ln)
}

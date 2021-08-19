package main

import (
	"bytes"
	"os"
	"os/signal"
	"syscall"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/nbio"
	nlog "github.com/lesismal/nbio/logging"
	"github.com/lesismal/nbio/mempool"
)

var (
	addr    = "localhost:8888"
	handler = arpc.NewHandler()
)

// HelloReq .
type HelloReq struct {
	Msg string
}

// HelloRsp .
type HelloRsp struct {
	Msg string
}

// Session .
type Session struct {
	*arpc.Client
	bytes.Buffer
}

func onOpen(c *nbio.Conn) {
	client := &arpc.Client{Conn: c, Codec: codec.DefaultCodec, Handler: handler}
	session := &Session{
		Client: client,
	}
	c.SetSession(session)
}

func onData(c *nbio.Conn, data []byte) {
	iSession := c.Session()
	if iSession == nil {
		c.Close()
		return
	}
	session := iSession.(*Session)
	session.Write(data)
	for session.Len() >= arpc.HeadLen {
		headBuf := session.Bytes()[:4]
		header := arpc.Header(headBuf)
		total := arpc.HeadLen + header.BodyLen()
		if session.Len() < total {
			return
		}

		buffer := mempool.Malloc(total)
		session.Read(buffer)

		msg := handler.NewMessageWithBuffer(buffer)
		handler.OnMessage(session.Client, msg)
	}
}

func main() {
	nlog.SetLogger(log.DefaultLogger)

	arpc.BufferPool = mempool.DefaultMemPool

	handler.EnablePool(true)
	handler.SetAsyncWrite(false)
	handler.SetAsyncResponse(true)

	// register router
	handler.Handle("Hello", func(ctx *arpc.Context) {
		req := &HelloReq{}
		ctx.Bind(req)
		ctx.Write(&HelloRsp{Msg: req.Msg})
	})

	g := nbio.NewGopher(nbio.Config{
		Network: "tcp",
		Addrs:   []string{addr},
	})

	g.OnOpen(onOpen)
	g.OnData(onData)

	err := g.Start()
	if err != nil {
		log.Error("Start failed: %v", err)
	}
	defer g.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

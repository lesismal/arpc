package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/nbio"
	nlog "github.com/lesismal/nbio/logging"
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
	Client *arpc.Client
	Buffer []byte
}

func onOpen(c *nbio.Conn) {
	client := &arpc.Client{Conn: c, Codec: codec.DefaultCodec, IsAsync: true}
	session := &Session{
		Client: client,
		Buffer: nil,
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
	session.Buffer = append(session.Buffer, data...)
	if len(session.Buffer) < arpc.HeadLen {
		return
	}

	headBuf := session.Buffer[:4]
	header := arpc.Header(headBuf)
	if len(session.Buffer) < arpc.HeadLen+header.BodyLen() {
		return
	}

	msg := &arpc.Message{Buffer: session.Buffer[:arpc.HeadLen+header.BodyLen()]}
	session.Buffer = session.Buffer[arpc.HeadLen+header.BodyLen():]
	handler.OnMessage(session.Client, msg)
}

func main() {
	nlog.SetLogger(log.DefaultLogger)

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

	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

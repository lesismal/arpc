package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/llib/bytes"
	"github.com/lesismal/nbio"
	nlog "github.com/lesismal/nbio/log"
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
	Buffer *bytes.Buffer
}

func onOpen(c *nbio.Conn) {
	client := &arpc.Client{Conn: c, Codec: codec.DefaultCodec}
	session := &Session{
		Client: client,
		Buffer: bytes.NewBuffer(),
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
	buffer := session.Buffer
	buffer.Push(append([]byte{}, data...))
	buf, err := buffer.Head(4)
	if err != nil {
		return
	}
	header := arpc.Header(buf)
	buf, err = buffer.Pop(arpc.HeadLen + header.BodyLen())
	if err != nil {
		return
	}

	handler.OnMessage(session.Client, &arpc.Message{Buffer: append([]byte{}, buf...)})
}

func main() {
	nlog.SetLogger(log.DefaultLogger)

	// register router
	handler.Handle("Hello", func(ctx *arpc.Context) {
		req := &HelloReq{}
		ctx.Bind(req)
		ctx.Write2(&HelloRsp{Msg: req.Msg})
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

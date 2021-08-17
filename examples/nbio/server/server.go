package main

import (
	"bytes"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

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

// Session .
type Session struct {
	bufMux sync.Mutex
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
	session.bufMux.Lock()
	session.Buffer.Write(data)
	session.bufMux.Unlock()

	c.Execute(func() {
		for {
			session.bufMux.Lock()
			if session.Len() < arpc.HeadLen {
				session.bufMux.Unlock()
				return
			}
			headBuf := session.Bytes()[:4]
			header := arpc.Header(headBuf)
			total := arpc.HeadLen + header.BodyLen()
			if session.Len() < total {
				session.bufMux.Unlock()
				return
			}

			buffer := mempool.Malloc(total)
			session.Buffer.Read(buffer)
			session.bufMux.Unlock()

			msg := handler.NewMessageWithBuffer(buffer)
			handler.OnMessage(session.Client, msg)
		}
	})
}

func main() {
	nlog.SetLogger(log.DefaultLogger)

	arpc.BufferPool = mempool.DefaultMemPool

	handler.EnablePool(true)
	handler.SetAsyncWrite(false)
	handler.SetAsyncResponse(true)

	// register router
	handler.Handle("/echo", func(ctx *arpc.Context) {
		str := ""
		err := ctx.Bind(&str)
		if err != nil {
			ctx.Error("invalid message")
			log.Error("Bind failed: %v", err)
			return
		}
		ctx.Write(str)
		log.Info("/echo: %v", str)
	})

	g := nbio.NewGopher(nbio.Config{
		Network: "tcp",
		Addrs:   []string{addr},
	})

	g.OnOpen(onOpen)
	g.OnData(onData)
	g.OnWriteBufferRelease(func(c *nbio.Conn, b []byte) {
		mempool.Free(b)
	})
	g.Execute = func(f func()) {
		go func() {
			defer func() {
				if err := recover(); err != nil {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]
					nlog.Error("Execute failed: %v\n%v\n", err, *(*string)(unsafe.Pointer(&buf)))
				}
			}()
			f()
		}()
	}

	err := g.Start()
	if err != nil {
		log.Error("Start failed: %v", err)
	}
	defer g.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

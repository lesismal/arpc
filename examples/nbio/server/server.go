package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/extension/listener"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/nbio"
	nlog "github.com/lesismal/nbio/logging"
	"github.com/lesismal/nbio/mempool"
	"github.com/lesismal/nbio/taskpool"
)

var (
	addr = "localhost:8888"

	multiListener *listener.Listener

	nbioHandler = arpc.NewHandler()

	stdSvr  = arpc.NewServer()
	nbioSvr = nbio.NewGopher(nbio.Config{})

	pool = taskpool.NewMixedPool(1024*8, 1, 1024*8)

	method = "/echo"
)

func onEcho(ctx *arpc.Context) {
	str := ""
	err := ctx.Bind(&str)
	if err != nil {
		ctx.Error("invalid message")
		log.Error("Bind failed: %v", err)
		return
	}
	ctx.Write(str)
}

func main() {
	nlog.SetLogger(log.DefaultLogger)

	arpc.BufferPool = mempool.DefaultMemPool

	var err error
	var maxStdOnline = 5
	multiListener, err = listener.Listen("tcp", addr, maxStdOnline, "")
	if err != nil {
		panic(err)
	}
	lnA, lnB := multiListener.Listeners()
	initStdServer(lnA)
	initNBIOServer(lnB)
	go multiListener.Run()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	multiListener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	nbioSvr.Stop()
	stdSvr.Shutdown(ctx)
}

func initStdServer(ln net.Listener) {
	stdSvr.Handler.EnablePool(true)
	stdSvr.Handler.SetAsyncResponse(true)
	stdSvr.Handler.HandleDisconnected(func(*arpc.Client) {
		multiListener.OfflineA()
	})

	// register router
	stdSvr.Handler.Handle(method, onEcho)

	go func() {
		err := stdSvr.Serve(ln)
		log.Error("stdSvr serve: %v", err)
	}()
}

func initNBIOServer(ln net.Listener) {
	nbioHandler.EnablePool(true)
	nbioHandler.SetAsyncWrite(false)

	// register router
	nbioHandler.Handle(method, onEcho)

	nbioSvr.OnOpen(nbioOnOpen)
	nbioSvr.OnData(nbioOnData)
	nbioSvr.OnClose(nbioOnClose)
	nbioSvr.Execute = pool.Go

	err := nbioSvr.Start()
	if err != nil {
		log.Error("Start failed: %v", err)
		panic(err)
	}

	go func() {
		n := 0
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			n++
			println("nbio.Gopher total Accepted:", n)
			nbioSvr.AddConn(c)
		}
	}()
}

// Session .
type Session struct {
	*arpc.Client
	cache []byte
}

func nbioOnOpen(c *nbio.Conn) {
	client := &arpc.Client{Conn: c, Codec: codec.DefaultCodec, Handler: nbioHandler}
	session := &Session{
		Client: client,
	}
	c.SetSession(session)
}

func nbioOnClose(c *nbio.Conn, err error) {
	iSession := c.Session()
	if iSession == nil {
		c.Close()
		return
	}
	c.MustExecute(func() {
		session := iSession.(*Session)
		if session.cache != nil {
			mempool.Free(session.cache)
			session.cache = nil
		}
	})
}

func nbioOnData(c *nbio.Conn, data []byte) {
	iSession := c.Session()
	if iSession == nil {
		c.Close()
		return
	}

	c.Execute(func() {
		session := iSession.(*Session)
		start := 0
		if session.cache != nil {
			session.cache = append(session.cache, data...)
			data = session.cache
		}
		for {
			if len(data) < arpc.HeadLen {
				goto Exit
			}
			header := arpc.Header(data[start : start+4])
			total := arpc.HeadLen + header.BodyLen()
			if len(data)-start < total {
				goto Exit
			}

			buffer := mempool.Malloc(total)
			copy(buffer, data[start:start+total])
			start += total
			msg := nbioHandler.NewMessageWithBuffer(buffer)
			pool.Go(func() {
				nbioHandler.OnMessage(session.Client, msg)
			})
		}

	Exit:
		if session.cache != nil {
			if start == len(data) {
				mempool.Free(data)
				session.cache = nil
			} else {
				left := mempool.Malloc(len(data) - start)
				copy(left, data[start:])
				mempool.Free(data)
				session.cache = left
			}
		} else if start < len(data) {
			left := mempool.Malloc(len(data) - start)
			copy(left, data[start:])
			mempool.Free(data)
			session.cache = left
		}
	})
}

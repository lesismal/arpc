// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"log"
	"math/rand"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lesismal/arpcext/websocket"
)

var (
	allAddr   = "localhost:16788"
	benchAddr = "localhost:16789"

	benchServer *Server
	benchClient *Client
)

func Benchmark_Call_String_Payload_64(b *testing.B) {
	benchmarkCallStringPayload(b, randString(64))
}

func Benchmark_Call_String_Payload_128(b *testing.B) {
	benchmarkCallStringPayload(b, randString(128))
}

func Benchmark_Call_String_Payload_256(b *testing.B) {
	benchmarkCallStringPayload(b, randString(256))
}

func Benchmark_Call_String_Payload_512(b *testing.B) {
	benchmarkCallStringPayload(b, randString(512))
}

func Benchmark_Call_String_Payload_1024(b *testing.B) {
	benchmarkCallStringPayload(b, randString(1024))
}

func Benchmark_Call_String_Payload_2048(b *testing.B) {
	benchmarkCallStringPayload(b, randString(2048))
}

func Benchmark_Call_String_Payload_4096(b *testing.B) {
	benchmarkCallStringPayload(b, randString(4096))
}

func Benchmark_Call_String_Payload_8192(b *testing.B) {
	benchmarkCallStringPayload(b, randString(8192))
}

func Benchmark_Call_Bytes_Payload_64(b *testing.B) {
	benchmarkCallBytesPayload(b, make([]byte, 64))
}

func Benchmark_Call_Bytes_Payload_128(b *testing.B) {
	benchmarkCallBytesPayload(b, make([]byte, 128))
}

func Benchmark_Call_Bytes_Payload_256(b *testing.B) {
	benchmarkCallBytesPayload(b, make([]byte, 256))
}

func Benchmark_Call_Bytes_Payload_512(b *testing.B) {
	benchmarkCallBytesPayload(b, make([]byte, 512))
}

func Benchmark_Call_Bytes_Payload_1024(b *testing.B) {
	benchmarkCallBytesPayload(b, make([]byte, 1024))
}

func Benchmark_Call_Bytes_Payload_2048(b *testing.B) {
	benchmarkCallBytesPayload(b, make([]byte, 2048))
}

func Benchmark_Call_Bytes_Payload_4096(b *testing.B) {
	benchmarkCallBytesPayload(b, make([]byte, 4096))
}

func Benchmark_Call_Bytes_Payload_8192(b *testing.B) {
	benchmarkCallBytesPayload(b, make([]byte, 8192))
}

func Benchmark_Call_Struct_Payload_64(b *testing.B) {
	benchmarkCallStructPayload(b, &message{Payload: randString(64)})
}

func Benchmark_Call_Struct_Payload_128(b *testing.B) {
	benchmarkCallStructPayload(b, &message{Payload: randString(128)})
}

func Benchmark_Call_Struct_Payload_256(b *testing.B) {
	benchmarkCallStructPayload(b, &message{Payload: randString(256)})
}

func Benchmark_Call_Struct_Payload_512(b *testing.B) {
	benchmarkCallStructPayload(b, &message{Payload: randString(512)})
}

func Benchmark_Call_Struct_Payload_1024(b *testing.B) {
	benchmarkCallStructPayload(b, &message{Payload: randString(1024)})
}

func Benchmark_Call_Struct_Payload_2048(b *testing.B) {
	benchmarkCallStructPayload(b, &message{Payload: randString(2048)})
}

func Benchmark_Call_Struct_Payload_4096(b *testing.B) {
	benchmarkCallStructPayload(b, &message{Payload: randString(4096)})
}

func Benchmark_Call_Struct_Payload_8192(b *testing.B) {
	benchmarkCallStructPayload(b, &message{Payload: randString(8192)})
}

func init() {
	SetLogger(nil)
	benchServer = newBenchServer()
	benchClient = newBenchClient()
}

type message struct {
	Payload string
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", benchAddr, time.Second)
}

func randString(n int) string {
	letterBytes := "/?:=&1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		ret[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(ret)
}

func newBenchServer() *Server {
	s := NewServer()
	s.Handler.Handle("/echo/string", func(ctx *Context) {
		src := ""
		err := ctx.Bind(&src)
		if err != nil {
			log.Fatalf("Bind failed: %v", err)
		}
		ctx.Write(src)
	})
	s.Handler.Handle("/echo/bytes", func(ctx *Context) {
		src := ""
		err := ctx.Bind(&src)
		if err != nil {
			log.Fatalf("Bind failed: %v", err)
		}
		ctx.Write(src)
	})
	s.Handler.Handle("/echo/struct", func(ctx *Context) {
		var src message
		err := ctx.Bind(&src)
		if err != nil {
			log.Fatalf("Bind failed: %v", err)
		}
		ctx.Write(&src)
	})
	go s.Run(benchAddr)
	time.Sleep(time.Second)
	return s
}

func newBenchClient() *Client {
	c, err := NewClient(dialer)
	if err != nil {
		log.Fatalf("NewClient() failed: %v", err)
	}
	c.Run()
	return c
}

func benchmarkCallStringPayload(b *testing.B, src string) {
	for i := 0; i < b.N; i++ {
		dst := ""
		if err := benchClient.Call("/echo/string", src, &dst, time.Second); err != nil {
			b.Fatalf("benchClient.Call() string error: %v\nsrc: %v\ndst: %v", err, src, dst)
		}
	}
}

func benchmarkCallBytesPayload(b *testing.B, src []byte) {
	for i := 0; i < b.N; i++ {
		var dst []byte
		if err := benchClient.Call("/echo/bytes", src, &dst, time.Second); err != nil {
			b.Fatalf("benchClient.Call() error: %v\nsrc: %v\ndst: %v", err, src, dst)
		}
	}
}

func benchmarkCallStructPayload(b *testing.B, src *message) {
	for i := 0; i < b.N; i++ {
		var dst message
		if err := benchClient.Call("/echo/struct", src, &dst, time.Second); err != nil {
			b.Fatalf("benchClient.Call() struct error: %v\nsrc: %v\ndst: %v", err, src, dst)
		}
	}
}

func TestClientPool(t *testing.T) {
	pool, err := NewClientPool(dialer, 2)
	if err != nil {
		log.Fatalf("NewClient() failed: %v", err)
	}
	if pool.Size() != 2 {
		t.Fatalf("invalid pool size: %v", pool.Size())
	}
	pool.Handler().Handle("/poolmethod", func(*Context) {})
	pool.Run()
	defer pool.Stop()

	var src = "test"
	var dst []byte
	if err = pool.Get(1).Call("/echo/bytes", src, &dst, time.Second); err != nil {
		t.Fatalf("pool.Call() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	if err = pool.Next().Call("/echo/bytes", src, &dst, time.Second); err != nil {
		t.Fatalf("pool.Call() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}

	pool2, err := NewClientPoolFromDialers([]DialerFunc{dialer, dialer})
	if err != nil {
		t.Fatalf("NewClientPoolFromDialers error: %v", err)
	}
	if pool2 != nil {
		pool2.Stop()
	}

	pool3, err := NewClientPoolFromDialers([]DialerFunc{})
	if err == nil {
		t.Fatalf("NewClientPoolFromDialers with invalid dialer num(<=0) should not be allowed")
	}
	if pool3 != nil {
		pool3.Stop()
	}
}

func newSvr() *Server {
	DefaultHandler = NewHandler()
	s := NewServer()
	s.Handler = s.Handler.Clone()
	s.Handler.Handle("/call", func(ctx *Context) {
		src := ""
		err := ctx.Bind(&src)
		if err != nil {
			log.Fatalf("Bind failed: %v", err)
		}
		ctx.Write(src)
		ctx.Done()
	})
	s.Handler.Handle("/callasync", func(ctx *Context) {
		src := ""
		err := ctx.Bind(&src)
		if err != nil {
			log.Fatalf("Bind failed: %v", err)
		}
		ctx.Write(src)
	}, true)
	s.Handler.Handle("/notify", func(ctx *Context) {
		src := ""
		err := ctx.Bind(&src)
		if err != nil {
			log.Fatalf("Bind failed: %v", err)
		}
		ctx.Write(src)
	})
	s.Handler.Handle("/timeout", func(ctx *Context) {
		src := ""
		err := ctx.Bind(&src)
		if err != nil {
			log.Fatalf("Bind failed: %v", err)
		}
		time.Sleep(time.Second / 10)
		ctx.Write(src)
	})
	s.Handler.Handle("/overstock", func(ctx *Context) {
		src := ""
		err := ctx.Bind(&src)
		if err != nil {
			log.Fatalf("Bind failed: %v", err)
		}
		ctx.Write(src)
	})
	go s.Run(allAddr)
	return s
}

func TestWebsocket(t *testing.T) {
	ln, _ := websocket.NewListener(":25341", nil)
	defer ln.Close()
	http.HandleFunc("/ws", ln.(*websocket.Listener).Handler)
	go func() {
		err := http.ListenAndServe(":25341", nil)
		if err != nil {
			t.Fatal("ListenAndServe: ", err)
		}
	}()

	svr := NewServer()
	svr.Handler.SetBatchRecv(false)
	// register router
	svr.Handler.Handle("/echo", func(ctx *Context) {
		str := ""
		ctx.Bind(&str)
		ctx.Write(str)
	})
	go svr.Serve(ln)

	time.Sleep(time.Second / 100)

	client, err := NewClient(func() (net.Conn, error) {
		return websocket.Dial("ws://localhost:25341/ws")
	})
	if err != nil {
		panic(err)
	}
	client.Handler.SetBatchRecv(false)

	client.Run()
	defer client.Stop()

	req := "hello"
	rsp := ""
	err = client.Call("/echo", &req, &rsp, time.Second*5)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
}

func TestClientNormal(t *testing.T) {
	var src = "test"
	var dst = ""
	var dstB []byte

	s := newSvr()
	defer s.Stop()
	time.Sleep(time.Second / 100)

	s.Handler.Use(func(ctx *Context) { ctx.Next() })
	c, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", allAddr, time.Second)
	})
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	c.Handler.SetBatchSend(false)
	c.Run()
	defer c.Stop()

	if err = c.Call("/call", src, &dst, time.Second); err != nil {
		t.Fatalf("Call() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	if err = c.Call("/call", src, &dstB, time.Second); err != nil {
		t.Fatalf("Call() error: %v\nsrc: %v\ndst: %v", err, src, dstB)
	}
	if err = c.CallWith(context.Background(), "/call", src, &dst); err != nil {
		t.Fatalf("CallWith() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	if err = c.CallWith(context.Background(), "/call", src, &dstB); err != nil {
		t.Fatalf("CallWith() error: %v\nsrc: %v\ndst: %v", err, src, dstB)
	}
	if err = c.CallAsync("/callasync", src, func(*Context) {}, time.Second); err != nil {
		t.Fatalf("CallAsync() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	if err = c.CallAsyncWith(context.Background(), "/callasync", src, func(*Context) {}); err != nil {
		t.Fatalf("Call() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	if err = c.Notify("/notify", src, time.Second); err != nil {
		t.Fatalf("Notify() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	if err = c.NotifyWith(context.Background(), "/notify", src); err != nil {
		t.Fatalf("NotifyWith() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	if err = c.Call("/timeout", src, &dst, time.Second/1000); err != ErrClientTimeout {
		t.Fatalf("Call() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	toCtx, cancel := context.WithTimeout(context.Background(), time.Second/1000)
	defer cancel()
	if err = c.CallWith(toCtx, "/timeout", src, &dst); err != ErrClientTimeout {
		t.Fatalf("CallWith() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
}

func TestClientError(t *testing.T) {
	var src = "test"
	var dst = ""
	var dstB []byte

	s := newSvr()
	defer s.Stop()
	time.Sleep(time.Second / 100)

	c, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", allAddr, time.Second)
	})
	if err != nil {
		log.Fatalf("NewClient() failed: %v", err)
	}

	if err = c.Call("/call", src, &dst, time.Second); err != ErrClientStopped {
		t.Fatalf("Call() error: %v\nsrc: %v\ndst: %v", err, src, dst)
	}
	if err = c.Call("/call", src, &dstB, time.Second); err != ErrClientStopped {
		t.Fatalf("Call() error: %v\nsrc: %v\ndst: %v", err, src, dstB)
	}

	invalidMethd := ""
	for i := 0; i < 128; i++ {
		invalidMethd += "a"
	}
	if err = c.Call(invalidMethd, src, &dstB, time.Second); err == nil {
		t.Fatalf("Call() invalid method error is nil")
	}
	if err = c.CallWith(context.Background(), invalidMethd, src, &dstB); err == nil {
		t.Fatalf("CallWith() invalid method error is nil")
	}
	if err = c.Notify(invalidMethd, src, time.Second); err == nil {
		t.Fatalf("Notify() invalid method error is nil")
	}
	if err = c.NotifyWith(context.Background(), invalidMethd, src); err == nil {
		t.Fatalf("NotifyWith() invalid method error is nil")
	}

	c.Handler.SetSendQueueSize(10)

	c.Run()
	defer c.Stop()

	c.Conn.Close()
	time.Sleep(time.Second / 100)

	c.Call("/call", src, &dst, time.Second)

	time.Sleep(time.Second)

	msg := NewMessage(CmdRequest, "/overstock", src, DefaultCodec)
	for i := 0; i < 10000; i++ {
		c.PushMsg(msg, 0)
	}
	c.Call("/overstock", src, &dst, 1)
	c.Call("/overstock", src, &dst, 0)
	c.Call("/nohandler", src, &dst, time.Second/100)
	c.PushMsg(msg, -1)
}

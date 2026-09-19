// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// Routes of the echo server, see startEchoServer.
const (
	routeEcho       = "/echo"
	routeEchoBytes  = "/echo/bytes"
	routeEchoStruct = "/echo/struct"
	routeError      = "/error"
	routeErrorValue = "/error/value"
	routeSleep      = "/sleep"
	routeNotify     = "/notify"
	routeStream     = "/stream"
)

// A config sets up the Handlers of both sides of a connection.
type config struct {
	name  string
	setup func(h Handler)
}

// writeModes are the ways a Handler reads and writes messages.
var writeModes = []config{
	{"Sync", func(h Handler) { h.SetAsyncWrite(false) }},
	{"SyncUnbufferedRecv", func(h Handler) { h.SetAsyncWrite(false); h.SetBatchRecv(false) }},
	{"Async", func(h Handler) {}},
	{"AsyncNoBatch", func(h Handler) { h.SetBatchSend(false) }},
	{"AsyncBatchBuffer", func(h Handler) { h.SetSendBufferSize(64) }},
	{"Writev", func(h Handler) { h.SetAsyncWritev(true) }},
}

// variants are features orthogonal to the write mode.
var variants = []config{
	{"Plain", func(h Handler) {}},
	{"Coder", func(h Handler) { h.UseCoder(xorCoder{}) }},
	{"Pool", func(h Handler) { h.EnablePool(true) }},
	{"SyncResponse", func(h Handler) { h.SetAsyncResponse(false) }},
}

// forEachConfig runs f as a subtest for every write mode and variant.
func forEachConfig(t *testing.T, f func(t *testing.T, setup func(h Handler))) {
	for _, mode := range writeModes {
		for _, variant := range variants {
			mode, variant := mode, variant
			t.Run(mode.name+"/"+variant.name, func(t *testing.T) {
				f(t, func(h Handler) {
					mode.setup(h)
					variant.setup(h)
				})
			})
		}
	}
}

// echoServer is a Server with the echo routes.
type echoServer struct {
	*Server
	addr     string
	notified chan string
}

// startEchoServer starts a Server set up by setup, with the echo routes.
func startEchoServer(t *testing.T, setup func(h Handler)) *echoServer {
	es := &echoServer{notified: make(chan string, 1024)}
	es.Server, es.addr = startServer(t, func(h Handler) {
		if setup != nil {
			setup(h)
		}
		h.Handle(routeEcho, func(ctx *Context) {
			var s string
			if err := ctx.Bind(&s); err != nil {
				ctx.Error(err)
				return
			}
			ctx.Write(s)
		})
		h.Handle(routeEchoBytes, func(ctx *Context) {
			ctx.Write(ctx.Body())
		})
		h.Handle(routeEchoStruct, func(ctx *Context) {
			var p payload
			if err := ctx.Bind(&p); err != nil {
				ctx.Error(err)
				return
			}
			p.A++
			ctx.Write(&p)
		})
		h.Handle(routeError, func(ctx *Context) {
			ctx.Error("remote error")
		})
		h.Handle(routeErrorValue, func(ctx *Context) {
			ctx.Write(errors.New("remote error value"))
		})
		h.Handle(routeSleep, func(ctx *Context) {
			time.Sleep(200 * time.Millisecond)
			ctx.Write("late")
		}, true)
		h.Handle(routeNotify, func(ctx *Context) {
			var s string
			ctx.Bind(&s)
			es.notified <- s
		})
		// Async so that the blocking Recv never runs on the read goroutine.
		h.HandleStream(routeStream, func(stream *Stream) {
			// Echo every message, then close when the peer does.
			for {
				var s string
				if err := stream.Recv(&s); err != nil {
					stream.CloseSend()
					return
				}
				stream.Send(s)
			}
		}, true)
	})
	return es
}

func TestRPC(t *testing.T) {
	forEachConfig(t, func(t *testing.T, setup func(h Handler)) {
		es := startEchoServer(t, setup)
		c := dialClient(t, es.addr, setup)

		t.Run("CallString", func(t *testing.T) {
			var rsp string
			if err := c.Call(routeEcho, "hello", &rsp, time.Second); err != nil || rsp != "hello" {
				t.Fatalf("Call = %v, %q", err, rsp)
			}
			req := "hello ptr"
			if err := c.Call(routeEcho, &req, &rsp, time.Second); err != nil || rsp != req {
				t.Fatalf("Call(*string) = %v, %q", err, rsp)
			}
			// Message values are local and not sent.
			values := map[interface{}]interface{}{"k": "v"}
			if err := c.Call(routeEcho, "values", &rsp, time.Second, values); err != nil || rsp != "values" {
				t.Fatalf("Call with values = %v, %q", err, rsp)
			}
		})

		t.Run("CallBytes", func(t *testing.T) {
			var rsp []byte
			if err := c.Call(routeEchoBytes, []byte("bytes"), &rsp, time.Second); err != nil || string(rsp) != "bytes" {
				t.Fatalf("Call = %v, %q", err, rsp)
			}
			req := []byte("bytes ptr")
			if err := c.Call(routeEchoBytes, &req, &rsp, time.Second); err != nil || string(rsp) != "bytes ptr" {
				t.Fatalf("Call(*[]byte) = %v, %q", err, rsp)
			}
		})

		t.Run("CallStruct", func(t *testing.T) {
			var rsp payload
			if err := c.Call(routeEchoStruct, &payload{A: 1, B: "b"}, &rsp, time.Second); err != nil || rsp != (payload{A: 2, B: "b"}) {
				t.Fatalf("Call = %v, %+v", err, rsp)
			}
		})

		t.Run("CallNilRsp", func(t *testing.T) {
			if err := c.Call(routeEcho, "x", nil, time.Second); err != nil {
				t.Fatalf("Call = %v", err)
			}
		})

		t.Run("CallErrors", func(t *testing.T) {
			var rsp string
			if err := c.Call(routeError, "x", &rsp, time.Second); err == nil || err.Error() != "remote error" {
				t.Fatalf("Call(error) = %v", err)
			}
			if err := c.Call(routeErrorValue, "x", &rsp, time.Second); err == nil || err.Error() != "remote error value" {
				t.Fatalf("Call(error value) = %v", err)
			}
			if err := c.Call("/not/found", "x", &rsp, time.Second); err == nil || err.Error() != ErrMethodNotFound.Error() {
				t.Fatalf("Call(not found) = %v", err)
			}
			var n int
			if err := c.Call(routeEcho, "x", &n, time.Second); err == nil {
				t.Fatal("Call decoding into a mismatched type should fail")
			}
		})

		t.Run("CallContext", func(t *testing.T) {
			var rsp string
			if err := c.CallContext(context.Background(), routeEcho, "ctx", &rsp); err != nil || rsp != "ctx" {
				t.Fatalf("CallContext = %v, %q", err, rsp)
			}
			if err := c.CallWith(context.Background(), routeEcho, "with", &rsp); err != nil || rsp != "with" {
				t.Fatalf("CallWith = %v, %q", err, rsp)
			}
		})

		t.Run("CallAsync", func(t *testing.T) {
			type result struct {
				data string
				err  error
			}
			done := make(chan result, 2)
			handler := func(ctx *Context, err error) {
				var s string
				if err == nil {
					err = ctx.Bind(&s)
				}
				done <- result{s, err}
			}
			if err := c.CallAsync(routeEcho, "async", handler, time.Second); err != nil {
				t.Fatalf("CallAsync = %v", err)
			}
			if r := recvWithin(t, done, "async response"); r.err != nil || r.data != "async" {
				t.Fatalf("async result = %+v", r)
			}
			if err := c.CallAsync(routeError, "x", handler, time.Second); err != nil {
				t.Fatalf("CallAsync = %v", err)
			}
			if r := recvWithin(t, done, "async error"); r.err == nil || r.err.Error() != "remote error" {
				t.Fatalf("async error result = %+v", r)
			}
		})

		t.Run("Notify", func(t *testing.T) {
			if err := c.Notify(routeNotify, "n1", time.Second); err != nil {
				t.Fatalf("Notify = %v", err)
			}
			if err := c.Notify(routeNotify, "n2", 0); err != nil {
				t.Fatalf("Notify(0) = %v", err)
			}
			if err := c.NotifyContext(context.Background(), routeNotify, "n3"); err != nil {
				t.Fatalf("NotifyContext = %v", err)
			}
			if err := c.NotifyWith(context.Background(), routeNotify, "n4"); err != nil {
				t.Fatalf("NotifyWith = %v", err)
			}
			// PushMsg sends a prepared message as is.
			if err := c.PushMsg(c.NewMessage(CmdNotify, routeNotify, "n5"), time.Second); err != nil {
				t.Fatalf("PushMsg = %v", err)
			}
			// Handlers may run concurrently, so the order is not kept.
			got := map[string]bool{}
			for i := 0; i < 5; i++ {
				got[recvWithin(t, es.notified, "notify")] = true
			}
			for _, want := range []string{"n1", "n2", "n3", "n4", "n5"} {
				if !got[want] {
					t.Fatalf("notified %v, missing %q", got, want)
				}
			}
		})

		t.Run("Stream", func(t *testing.T) {
			stream := c.NewStream(routeStream)
			for _, s := range []string{"s1", "s2", "s3"} {
				if err := stream.Send(s); err != nil {
					t.Fatalf("Send = %v", err)
				}
				var got string
				if err := stream.Recv(&got); err != nil || got != s {
					t.Fatalf("Recv = %v, %q, want %q", err, got, s)
				}
			}
			if err := stream.SendAndClose("last"); err != nil {
				t.Fatalf("SendAndClose = %v", err)
			}
			var got string
			if err := stream.Recv(&got); err != nil || got != "last" {
				t.Fatalf("Recv = %v, %q", err, got)
			}
			if err := stream.Recv(&got); err == nil {
				t.Fatal("Recv after the peer closed should fail")
			}
		})

		t.Run("Concurrent", func(t *testing.T) {
			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					req := payload{A: i, B: "concurrent"}
					var rsp payload
					if err := c.Call(routeEchoStruct, &req, &rsp, 2*time.Second); err != nil || rsp.A != i+1 {
						t.Errorf("Call %v = %v, %+v", i, err, rsp)
					}
				}(i)
			}
			wg.Wait()
		})
	})
}

func TestRPC_ServerToClient(t *testing.T) {
	// A server-role Client can call the client-role one on the same conn.
	forEachConfig(t, func(t *testing.T, setup func(h Handler)) {
		svrClients := make(chan *Client, 1)
		_, addr := startServer(t, func(h Handler) {
			setup(h)
			h.HandleConnected(func(c *Client) { svrClients <- c })
		})
		dialClient(t, addr, func(h Handler) {
			setup(h)
			h.Handle(routeEcho, func(ctx *Context) {
				var s string
				ctx.Bind(&s)
				ctx.Write("client:" + s)
			})
		})
		sc := recvWithin(t, svrClients, "server-side client")

		var rsp string
		if err := sc.Call(routeEcho, "hi", &rsp, time.Second); err != nil || rsp != "client:hi" {
			t.Fatalf("server Call = %v, %q", err, rsp)
		}
	})
}

func TestRPC_Timeout(t *testing.T) {
	for _, mode := range writeModes {
		mode := mode
		t.Run(mode.name, func(t *testing.T) {
			es := startEchoServer(t, mode.setup)
			var missed int32
			var mu sync.Mutex
			c := dialClient(t, es.addr, func(h Handler) {
				mode.setup(h)
				h.HandleSessionMiss(func(c *Client, m *Message) {
					mu.Lock()
					missed++
					mu.Unlock()
				})
			})

			var rsp string
			if err := c.Call(routeSleep, "x", &rsp, 20*time.Millisecond); err != ErrClientTimeout {
				t.Fatalf("Call = %v, want ErrClientTimeout", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if err := c.CallContext(ctx, routeSleep, "x", &rsp); err != ErrClientTimeout {
				t.Fatalf("CallContext = %v, want ErrClientTimeout", err)
			}

			done := make(chan error, 1)
			if err := c.CallAsync(routeSleep, "x", func(ctx *Context, err error) {
				if ctx != nil {
					t.Errorf("ctx should be nil on timeout")
				}
				done <- err
			}, 20*time.Millisecond); err != nil {
				t.Fatalf("CallAsync = %v", err)
			}
			if err := recvWithin(t, done, "async timeout"); err != ErrTimeout {
				t.Fatalf("async err = %v, want ErrTimeout", err)
			}

			// The late responses arrive after their calls are gone.
			waitFor(t, "session misses", func() bool {
				mu.Lock()
				defer mu.Unlock()
				return missed == 3
			})
			// The async handler is not called again for the late response.
			assertNoRecv(t, done, 50*time.Millisecond, "second async callback")
		})
	}
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesismal/arpc/codec"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler()
	if h.LogTag() != "[ARPC CLI]" || !h.BatchRecv() || !h.BatchSend() || !h.AsyncWrite() || h.AsyncWritev() ||
		!h.AsyncResponse() || h.RecvBufferSize() != 8192 || h.SendBufferSize() != 0 || h.SendQueueSize() != 4096 ||
		h.StreamQueueSize() != 4 || h.MaxBodyLen() != DefaultMaxBodyLen || h.MaxReconnectTimes() != 0 ||
		h.ReadTimeout() != 0 || h.WriteTimeout() != 0 || len(h.Coders()) != 0 {
		t.Fatalf("unexpected defaults: %+v", h)
	}
	if ctx, cancel := h.Context(); ctx == nil || cancel == nil {
		t.Fatal("Context should be set")
	}

	// The default WrapReader buffers the conn.
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, ok := h.WrapReader(a).(*bufio.Reader); !ok {
		t.Fatal("WrapReader should return a bufio.Reader")
	}
	h.SetReaderWrapper(nil)
	if h.WrapReader(a) != a {
		t.Fatal("WrapReader without a wrapper should return the conn")
	}

	// The default OnConnected disables TCP_NODELAY and tolerates other conns.
	h.OnConnected(&Client{Conn: a})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	h.OnConnected(&Client{Conn: conn})
}

func TestHandler_Settings(t *testing.T) {
	h := NewHandler()
	h.SetLogTag("tag")
	h.SetBatchRecv(false)
	h.SetBatchSend(false)
	h.SetAsyncWrite(false)
	h.SetAsyncWritev(true)
	h.SetAsyncResponse(false)
	h.SetRecvBufferSize(1)
	h.SetSendBufferSize(2)
	h.SetSendQueueSize(3)
	h.SetStreamQueueSize(4)
	h.SetMaxBodyLen(5)
	h.SetMaxReconnectTimes(6)
	h.SetReadTimeout(7)
	h.SetWriteTimeout(8)
	if h.LogTag() != "tag" || h.BatchRecv() || h.BatchSend() || h.AsyncWrite() || !h.AsyncWritev() ||
		h.AsyncResponse() || h.RecvBufferSize() != 1 || h.SendBufferSize() != 2 || h.SendQueueSize() != 3 ||
		h.StreamQueueSize() != 4 || h.MaxBodyLen() != 5 || h.MaxReconnectTimes() != 6 ||
		h.ReadTimeout() != 7 || h.WriteTimeout() != 8 {
		t.Fatalf("settings not applied: %+v", h)
	}
}

func TestHandler_Callbacks(t *testing.T) {
	h := NewHandler()
	c := &Client{}
	m := &Message{}

	// Without callbacks, the On* methods do nothing.
	h.HandleConnected(nil)
	h.OnConnected(c)
	h.OnDisconnected(c)
	h.OnReconnect(c, &ReconnectInfo{})
	h.OnOverstock(c, m)
	h.OnMessageDropped(c, m)
	h.OnMessageDone(c, m)
	h.OnSessionMiss(c, m)
	h.OnContextDone(&Context{})

	var calls []string
	record := func(name string) { calls = append(calls, name) }
	h.HandleConnected(func(*Client) { record("connected") })
	h.HandleDisconnected(func(*Client) { record("disconnected") })
	h.HandleReconnect(func(_ *Client, info *ReconnectInfo) { record("reconnect") })
	h.HandleMessageDone(func(*Client, *Message) { record("done") })
	h.HandleSessionMiss(func(*Client, *Message) { record("miss") })
	h.HandleContextDone(func(*Context) { record("ctxdone") })

	h.OnConnected(c)
	h.OnDisconnected(c)
	h.OnReconnect(c, &ReconnectInfo{})
	h.OnContextDone(&Context{})
	h.OnMessageDone(c, nil) // no-op for a nil message
	// OnSessionMiss is followed by OnMessageDone.
	h.OnSessionMiss(c, m)

	// OnOverstock and OnMessageDropped are followed by OnMessageDone, even
	// when registered with a nil func.
	h.HandleOverstock(func(*Client, *Message) { record("overstock") })
	h.OnOverstock(c, m)
	h.HandleOverstock(nil)
	h.OnOverstock(c, m)
	h.HandleMessageDropped(func(*Client, *Message) { record("dropped") })
	h.OnMessageDropped(c, m)
	h.HandleMessageDropped(nil)
	h.OnMessageDropped(c, m)

	want := "connected disconnected reconnect ctxdone miss done overstock done done dropped done done"
	if got := strings.Join(calls, " "); got != want {
		t.Fatalf("calls:\n%v\nwant:\n%v", got, want)
	}
}

func TestHandler_Clone(t *testing.T) {
	h := NewHandler()
	h.SetLogTag("orig")
	h.Use(func(*Context) {})
	h.UseCoder(xorCoder{})
	h.Handle("/a", func(*Context) {})
	h.HandleStream("/s", func(*Stream) {})
	h.Singleflight("/a")

	cp := h.Clone()
	origCtx, _ := h.Context()
	cpCtx, _ := cp.Context()
	if cp.LogTag() != "orig" || len(cp.Coders()) != 1 || origCtx == cpCtx {
		t.Fatal("Clone should copy the settings with a new context")
	}
	if _, ok := cp.SingleflightKey("/a", 1); !ok {
		t.Fatal("Clone should copy singleflights")
	}

	// Changes to the clone do not affect the original.
	cp.Use(func(*Context) {})
	cp.UseCoder(xorCoder{})
	cp.Handle("/b", func(*Context) {})
	cp.HandleStream("/t", func(*Stream) {})
	cp.Singleflight("/b")
	o, c := h.(*handler), cp.(*handler)
	if len(o.middles) != 1 || len(o.msgCoders) != 1 || len(o.routes["/a"].handlers) != 2 ||
		o.routes["/b"] != nil || o.streams["/t"] != nil || o.singleflights["/b"] != nil {
		t.Fatal("changes to the clone leaked into the original")
	}
	if len(c.routes["/a"].handlers) != 3 || c.streams["/s"] == nil {
		t.Fatal("clone routes not updated")
	}
}

func TestHandler_HandlePanics(t *testing.T) {
	h := NewHandler()
	long := strings.Repeat("m", MaxMethodLen+1)
	mustPanic(t, "empty method", func() { h.Handle("", func(*Context) {}) })
	mustPanic(t, "long method", func() { h.Handle(long, func(*Context) {}) })
	h.Handle("/a", func(*Context) {})
	mustPanic(t, "duplicate method", func() { h.Handle("/a", func(*Context) {}) })

	mustPanic(t, "long stream method", func() { h.HandleStream(long, func(*Stream) {}) })
	h.HandleStream("/s", func(*Stream) {})
	mustPanic(t, "duplicate stream method", func() { h.HandleStream("/s", func(*Stream) {}) })

	mustPanic(t, "empty singleflight method", func() { h.Singleflight("") })

	// Re-registering the not-found handler is allowed.
	h.HandleNotFound(func(*Context) {})
	h.HandleNotFound(func(*Context) {})
}

func TestHandler_HandleAsyncArg(t *testing.T) {
	h := NewHandler().(*handler)
	h.Handle("/default", func(*Context) {})
	h.Handle("/sync", func(*Context) {}, false)
	h.Handle("/async", func(*Context) {}, true)
	h.Handle("/ignored", func(*Context) {}, "not a bool")
	h.HandleStream("/s/default", func(*Stream) {})
	h.HandleStream("/s/sync", func(*Stream) {}, false)
	h.HandleStream("/s/ignored", func(*Stream) {}, 1)
	if !h.routes["/default"].async || h.routes["/sync"].async || !h.routes["/async"].async || !h.routes["/ignored"].async {
		t.Fatal("unexpected route async settings")
	}
	if !h.streams["/s/default"].async || h.streams["/s/sync"].async || !h.streams["/s/ignored"].async {
		t.Fatal("unexpected stream async settings")
	}
	h.SetAsyncResponse(false)
	h.Handle("/after", func(*Context) {})
	if h.routes["/after"].async {
		t.Fatal("SetAsyncResponse should change the default")
	}
}

func TestHandler_Middleware(t *testing.T) {
	trace := make(chan string, 16)
	_, addr := startServer(t, func(h Handler) {
		h.Use(nil) // ignored
		h.Use(func(ctx *Context) { trace <- "before" })
		h.Use(func(ctx *Context) {
			// Wrap the rest of the chain.
			trace <- "wrap in"
			ctx.Next()
			trace <- "wrap out"
		})
		h.Handle("/a", func(ctx *Context) {
			trace <- "handler"
			ctx.Write("ok")
		})
		h.Handle("/abort", func(ctx *Context) {
			trace <- "abort"
			ctx.Write("aborted")
			ctx.Abort()
		})
		// Middlewares added later run after the handlers already registered.
		h.Use(func(ctx *Context) { trace <- "after" })
		h.HandleNotFound(func(ctx *Context) {
			trace <- "notfound"
			ctx.Error("custom not found")
		})
	})
	c := dialClient(t, addr, nil)

	expect := func(want ...string) {
		t.Helper()
		for _, w := range want {
			if got := recvWithin(t, trace, w); got != w {
				t.Fatalf("got %q, want %q", got, w)
			}
		}
		assertNoRecv(t, trace, 10*time.Millisecond, "extra trace")
	}

	var rsp string
	if err := c.Call("/a", nil, &rsp, time.Second); err != nil || rsp != "ok" {
		t.Fatalf("Call = %v, %q", err, rsp)
	}
	expect("before", "wrap in", "handler", "after", "wrap out")

	if err := c.Call("/abort", nil, &rsp, time.Second); err != nil || rsp != "aborted" {
		t.Fatalf("Call = %v, %q", err, rsp)
	}
	expect("before", "wrap in", "abort", "wrap out")

	// The not-found handler registered last runs after all middlewares.
	if err := c.Call("/missing", nil, &rsp, time.Second); err == nil || err.Error() != "custom not found" {
		t.Fatalf("Call = %v", err)
	}
	expect("before", "wrap in", "after", "notfound", "wrap out")
}

func TestHandler_UseCoder(t *testing.T) {
	h := NewHandler()
	h.UseCoder(nil)
	if len(h.Coders()) != 0 {
		t.Fatal("UseCoder(nil) should be ignored")
	}
	h.UseCoder(xorCoder{})
	if len(h.Coders()) != 1 {
		t.Fatal("UseCoder should append")
	}
}

func TestHandler_Singleflight(t *testing.T) {
	h := NewHandler()
	if _, ok := h.SingleflightKey("/x", 1); ok {
		t.Fatal("singleflight should be off by default")
	}
	h.Singleflight("/x")
	h.Singleflight("/nil", nil)
	h.Singleflight("/custom", func(req interface{}) string { return "custom" })

	for _, tc := range []struct {
		method string
		req    interface{}
		key    string
	}{
		{"/x", 42, "42"},
		{"/x", &sfReq{ID: 7}, "sfReq:7"}, // fmt.Stringer
		{"/nil", "s", "s"},
		{"/custom", 1, "custom"},
	} {
		if key, ok := h.SingleflightKey(tc.method, tc.req); !ok || key != tc.key {
			t.Fatalf("SingleflightKey(%v, %v) = %q, %v, want %q", tc.method, tc.req, key, ok, tc.key)
		}
	}
	if _, ok := h.SingleflightKey("/other", 1); ok {
		t.Fatal("singleflight should be off for other methods")
	}
}

// rawClient returns a Client with the given Handler reading from one end of a
// net.Pipe, without running its loops, to call Handler.Recv directly.
func rawClient(t *testing.T, h Handler) (*Client, *peer) {
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	c := &Client{Conn: a, Handler: h, Codec: codec.DefaultCodec, Head: make([]byte, 4)}
	c.initReader()
	return c, &peer{t: t, conn: b}
}

func TestHandler_Recv(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		h := NewHandler()
		h.SetReadTimeout(time.Second)
		c, p := rawClient(t, h)
		go p.write(newMessage(CmdNotify, "m", "data", false, false, 3, nil, nil, nil).Buffer)
		msg, err := h.Recv(c)
		if err != nil || msg.Cmd() != CmdNotify || msg.Method() != "m" || string(msg.Data()) != "data" || msg.Seq() != 3 {
			t.Fatalf("Recv = %v, %v", msg, err)
		}
	})

	t.Run("BeforeRecvError", func(t *testing.T) {
		h := NewHandler()
		errBefore := errors.New("before recv")
		h.BeforeRecv(func(net.Conn) error { return errBefore })
		c, _ := rawClient(t, h)
		if _, err := h.Recv(c); err != errBefore {
			t.Fatalf("Recv = %v", err)
		}
	})

	t.Run("ReadTimeout", func(t *testing.T) {
		h := NewHandler()
		h.SetReadTimeout(10 * time.Millisecond)
		c, _ := rawClient(t, h)
		var ne net.Error
		if _, err := h.Recv(c); !errors.As(err, &ne) || !ne.Timeout() {
			t.Fatalf("Recv = %v, want a timeout", err)
		}
	})

	t.Run("BodyTooLong", func(t *testing.T) {
		h := NewHandler()
		h.SetMaxBodyLen(4)
		c, p := rawClient(t, h)
		go p.write(newMessage(CmdNotify, "m", "data", false, false, 1, nil, nil, nil).Buffer)
		if _, err := h.Recv(c); err == nil {
			t.Fatal("Recv of a too long body should fail")
		}
	})

	t.Run("ShortBody", func(t *testing.T) {
		h := NewHandler()
		c, p := rawClient(t, h)
		go func() {
			p.write(newMessage(CmdNotify, "m", "data", false, false, 1, nil, nil, nil).Buffer[:10])
			p.conn.Close()
		}()
		if _, err := h.Recv(c); err == nil {
			t.Fatal("Recv of a truncated message should fail")
		}
	})
}

func TestHandler_Send(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go io.Copy(io.Discard, b)

	h := NewHandler()
	h.SetWriteTimeout(time.Second)
	if n, err := h.Send(a, []byte("abc")); err != nil || n != 3 {
		t.Fatalf("Send = %v, %v", n, err)
	}
	if n, err := h.SendN(a, net.Buffers{[]byte("ab"), []byte("cd")}); err != nil || n != 4 {
		t.Fatalf("SendN = %v, %v", n, err)
	}

	errBefore := errors.New("before send")
	h.BeforeSend(func(net.Conn) error { return errBefore })
	if n, err := h.Send(a, []byte("abc")); err != errBefore || n != -1 {
		t.Fatalf("Send = %v, %v", n, err)
	}
	if n, err := h.SendN(a, net.Buffers{[]byte("ab")}); err != errBefore || n != -1 {
		t.Fatalf("SendN = %v, %v", n, err)
	}
}

func TestHandler_OnMessage(t *testing.T) {
	newPeerClient := func(t *testing.T, setup func(h Handler)) (*Client, *peer) {
		h := NewHandler()
		if setup != nil {
			setup(h)
		}
		c, _, p := pipeClient(t, h)
		return c, p
	}
	request := func(cmd byte, method string, seq uint64) []byte {
		return newMessage(cmd, method, "req", false, false, seq, nil, nil, nil).Buffer
	}

	t.Run("SyncHandler", func(t *testing.T) {
		_, p := newPeerClient(t, func(h Handler) {
			h.Handle("/sync", func(ctx *Context) { ctx.Write("sync") }, false)
		})
		p.write(request(CmdRequest, "/sync", 5))
		if m := p.read(); m.Cmd() != CmdResponse || m.Seq() != 5 || string(m.Data()) != "sync" {
			t.Fatalf("unexpected response cmd=%v seq=%v data=%q", m.Cmd(), m.Seq(), m.Data())
		}
	})

	t.Run("HandlerPanic", func(t *testing.T) {
		// A panic in a handler on the read goroutine is recovered.
		_, p := newPeerClient(t, func(h Handler) {
			h.Handle("/panic", func(ctx *Context) { panic("boom") }, false)
			h.Handle("/ok", func(ctx *Context) { ctx.Write("ok") }, false)
		})
		p.write(request(CmdRequest, "/panic", 1))
		p.write(request(CmdRequest, "/ok", 2))
		if m := p.read(); m.Seq() != 2 || string(m.Data()) != "ok" {
			t.Fatalf("unexpected response seq=%v data=%q", m.Seq(), m.Data())
		}
	})

	t.Run("InvalidMethodLen", func(t *testing.T) {
		_, p := newPeerClient(t, func(h Handler) {
			h.Handle("/ok", func(ctx *Context) { ctx.Write("ok") }, false)
		})
		// Method length 0, and longer than the body: both dropped.
		bad := newMessage(CmdRequest, "", "req", false, false, 1, nil, nil, nil)
		p.write(bad.Buffer)
		bad = newMessage(CmdRequest, "/x", "", false, false, 2, nil, nil, nil)
		bad.SetMethodLen(10)
		p.write(bad.Buffer)
		p.write(request(CmdRequest, "/ok", 3))
		if m := p.read(); m.Seq() != 3 {
			t.Fatalf("got a response to seq %v, want only 3", m.Seq())
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		// The default not-found route is added along with the first route.
		_, p := newPeerClient(t, func(h Handler) { h.Handle("/a", func(*Context) {}) })
		// A notify to an unknown method gets no response; a request gets
		// ErrMethodNotFound.
		p.write(request(CmdNotify, "/missing", 1))
		p.write(request(CmdRequest, "/missing", 2))
		m := p.read()
		if m.Seq() != 2 || !m.IsError() || m.Error().Error() != ErrMethodNotFound.Error() {
			t.Fatalf("unexpected response seq=%v err=%v", m.Seq(), m.Error())
		}
	})

	t.Run("SessionMiss", func(t *testing.T) {
		missed := make(chan uint64, 2)
		_, p := newPeerClient(t, func(h Handler) {
			h.HandleSessionMiss(func(c *Client, m *Message) { missed <- m.Seq() })
		})
		p.write(newMessage(CmdResponse, "/m", "rsp", false, false, 100, nil, nil, nil).Buffer)
		p.write(newMessage(CmdResponse, "/m", "rsp", false, true, 101, nil, nil, nil).Buffer)
		if seq := recvWithin(t, missed, "miss"); seq != 100 {
			t.Fatalf("missed seq %v", seq)
		}
		if seq := recvWithin(t, missed, "async miss"); seq != 101 {
			t.Fatalf("missed seq %v", seq)
		}
	})

	t.Run("StreamWithoutHandler", func(t *testing.T) {
		done := make(chan byte, 2)
		_, p := newPeerClient(t, func(h Handler) {
			h.HandleMessageDone(func(c *Client, m *Message) { done <- m.Cmd() })
		})
		// Neither a new remote Stream without handler nor a message for an
		// unknown local Stream is handled.
		msg := newMessage(CmdStream, "/nostream", "x", false, false, 1, nil, nil, nil)
		p.write(msg.Buffer)
		msg = newMessage(CmdStream, "/nostream", "x", false, false, 2, nil, nil, nil)
		msg.SetStreamLocal(true)
		p.write(msg.Buffer)
		recvWithin(t, done, "message done")
		recvWithin(t, done, "message done")
	})

	t.Run("InvalidCmd", func(t *testing.T) {
		disconnected := make(chan struct{}, 1)
		c, p := newPeerClient(t, func(h Handler) {
			h.HandleDisconnected(func(*Client) { disconnected <- struct{}{} })
		})
		msg := newMessage(CmdRequest, "/m", "x", false, false, 1, nil, nil, nil)
		msg.SetCmd(0x3F)
		p.write(msg.Buffer)
		recvWithin(t, disconnected, "stop on an invalid cmd")
		if clientRunning(c) {
			t.Fatal("the Client should stop")
		}
	})
}

func TestHandler_Alloc(t *testing.T) {
	h := NewHandler()
	if b := h.Malloc(3); len(b) != 3 {
		t.Fatalf("Malloc = %v", b)
	}
	if b := h.Append([]byte("a"), 'b'); string(b) != "ab" {
		t.Fatalf("Append = %q", b)
	}
	h.Free(nil)

	var mallocs, appends, frees int
	h.HandleMalloc(func(n int) []byte { mallocs++; return make([]byte, n) })
	h.HandleAppend(func(b []byte, more ...byte) []byte { appends++; return append(b, more...) })
	h.HandleFree(func([]byte) { frees++ })
	h.Malloc(1)
	h.Append(nil, 1)
	h.Free(nil)
	if mallocs != 1 || appends != 1 || frees != 1 {
		t.Fatalf("custom funcs called %v %v %v times", mallocs, appends, frees)
	}
}

func TestHandler_EnablePool(t *testing.T) {
	h := NewHandler()
	h.EnablePool(true)
	if b := h.Append(h.Malloc(1), 'x'); len(b) != 2 {
		t.Fatalf("pooled Append = %v", b)
	}
	// Messages and Contexts are released when done.
	msg := newMessage(CmdRequest, "m", "x", false, false, 1, h, nil, nil)
	ctx := newContext(&Client{}, msg, nil)
	h.OnContextDone(ctx)
	if ctx.Message != nil {
		t.Fatal("OnContextDone should release the Context")
	}
	msg = newMessage(CmdRequest, "m", "x", false, false, 1, h, nil, nil)
	h.OnMessageDone(nil, msg)
	if msg.Buffer != nil {
		t.Fatal("OnMessageDone should release the Message")
	}

	h.EnablePool(false)
	if b := h.Append(h.Malloc(1), 'x'); len(b) != 2 {
		t.Fatalf("Append = %v", b)
	}
	h.Free(nil)
	msg = newMessage(CmdRequest, "m", "x", false, false, 1, h, nil, nil)
	ctx = newContext(&Client{}, msg, nil)
	h.OnContextDone(ctx)
	h.OnMessageDone(nil, msg)
	if ctx.Message == nil || msg.Buffer == nil {
		t.Fatal("without the pool nothing is released")
	}
}

func TestHandler_Context(t *testing.T) {
	h := NewHandler()
	ctx, _ := h.Context()
	h.Cancel()
	if ctx.Err() == nil {
		t.Fatal("Cancel should cancel the context")
	}

	h.SetContext(nil, nil)
	h.Cancel() // no cancel func
	newCtx, cancel := context.WithCancel(context.Background())
	h.SetContext(newCtx, cancel)
	if got, _ := h.Context(); got != newCtx {
		t.Fatal("SetContext not applied")
	}
	h.Cancel()
	if newCtx.Err() == nil {
		t.Fatal("Cancel should cancel the new context")
	}
}

func TestHandler_NewMessage(t *testing.T) {
	h := NewHandler()
	msg := h.NewMessage(CmdRequest, "m", "x", true, true, 9, codec.DefaultCodec, map[interface{}]interface{}{"k": 1})
	if msg.Cmd() != CmdRequest || msg.Seq() != 9 || msg.IsError() || msg.IsAsync() || msg.handler != h {
		t.Fatal("unexpected message")
	}
	if v, _ := msg.Get("k"); v != 1 {
		t.Fatal("values not attached")
	}

	buf := append([]byte{}, msg.Buffer...)
	wrapped := h.NewMessageWithBuffer(buf)
	if wrapped.Method() != "m" || string(wrapped.Data()) != "x" || wrapped.handler != h {
		t.Fatal("NewMessageWithBuffer should wrap the buffer")
	}
}

func TestHandler_AsyncExecute(t *testing.T) {
	h := NewHandler()
	done := make(chan struct{})
	h.AsyncExecute(func() { close(done) })
	recvWithin(t, done, "default executor")

	var executed int32
	h.SetAsyncExecutor(func(f func()) {
		atomic.AddInt32(&executed, 1)
		f()
	})
	ran := false
	h.AsyncExecute(func() { ran = true })
	if !ran || executed != 1 {
		t.Fatal("custom executor not used")
	}
}

func TestDefaultHandlerFuncs(t *testing.T) {
	withDefaultHandler(t, func(h Handler) {
		SetLogTag("tag")
		SetBatchRecv(false)
		SetBatchSend(false)
		SetAsyncResponse(false)
		SetRecvBufferSize(1)
		SetSendBufferSize(2)
		SetSendQueueSize(3)
		SetStreamQueueSize(4)
		SetMaxBodyLen(5)
		SetReadTimeout(6)
		SetWriteTimeout(7)
		if h.LogTag() != "tag" || BatchRecv() || BatchSend() || AsyncResponse() || RecvBufferSize() != 1 ||
			SendBufferSize() != 2 || SendQueueSize() != 3 || StreamQueueSize() != 4 || MaxBodyLen() != 5 ||
			ReadTimeout() != 6 || WriteTimeout() != 7 {
			t.Fatal("package-level settings not applied to DefaultHandler")
		}

		var calls []string
		record := func(name string) { calls = append(calls, name) }
		HandleConnected(func(*Client) { record("connected") })
		HandleDisconnected(func(*Client) { record("disconnected") })
		HandleReconnect(func(*Client, *ReconnectInfo) { record("reconnect") })
		HandleOverstock(func(*Client, *Message) { record("overstock") })
		HandleMessageDropped(func(*Client, *Message) { record("dropped") })
		HandleSessionMiss(func(*Client, *Message) { record("miss") })
		HandleMalloc(func(n int) []byte { record("malloc"); return make([]byte, n) })
		HandleFree(func([]byte) { record("free") })
		SetAsyncExecutor(func(f func()) { record("execute"); f() })
		BeforeRecv(func(net.Conn) error { record("beforerecv"); return nil })
		BeforeSend(func(net.Conn) error { record("beforesend"); return nil })
		SetReaderWrapper(func(conn net.Conn) io.Reader { record("wrap"); return conn })

		c := &Client{}
		h.OnConnected(c)
		h.OnDisconnected(c)
		h.OnReconnect(c, nil)
		h.OnOverstock(c, nil)
		h.OnMessageDropped(c, nil)
		h.OnSessionMiss(c, nil)
		h.Malloc(1)
		h.Free(nil)
		AsyncExecute(func() {})
		h.WrapReader(nil)
		want := "connected disconnected reconnect overstock dropped miss malloc free execute wrap"
		if got := strings.Join(calls, " "); got != want {
			t.Fatalf("calls:\n%v\nwant:\n%v", got, want)
		}
		if h.(*handler).beforeRecv == nil || h.(*handler).beforeSend == nil {
			t.Fatal("BeforeRecv/BeforeSend not applied")
		}

		Use(func(*Context) {})
		UseCoder(xorCoder{})
		Handle("/a", func(*Context) {})
		HandleNotFound(func(*Context) {})
		Singleflight("/a")
		if err := Register("Calc", &calcService{}); err != nil {
			t.Fatalf("Register: %v", err)
		}
		EnablePool(true)
		EnablePool(false)
		hh := h.(*handler)
		if len(hh.middles) != 1 || len(hh.msgCoders) != 1 || hh.routes["/a"] == nil || hh.routes["Calc.Add"] == nil {
			t.Fatal("package-level routes not applied to DefaultHandler")
		}
		if _, ok := h.SingleflightKey("/a", 1); !ok {
			t.Fatal("Singleflight not applied")
		}
	})
}

func TestHeaderBodyLenEncoding(t *testing.T) {
	// The wire header is little endian, as documented.
	msg := newMessage(CmdNotify, "ab", "cde", false, false, 0x0102, nil, nil, nil)
	if binary.LittleEndian.Uint32(msg.Buffer[0:4]) != 5 || binary.LittleEndian.Uint64(msg.Buffer[8:16]) != 0x0102 {
		t.Fatal("unexpected header encoding")
	}
}

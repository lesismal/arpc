// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_Role(t *testing.T) {
	c := &Client{}
	if c.IsClient() || !c.IsServer() {
		t.Fatal("a Client without Dialer is server-role")
	}
	c.Dialer = func() (net.Conn, error) { return nil, nil }
	if !c.IsClient() || c.IsServer() {
		t.Fatal("a Client with Dialer is client-role")
	}
}

func TestClient_Values(t *testing.T) {
	c := &Client{}
	if _, ok := c.Get("k"); ok {
		t.Fatal("Get on empty values should fail")
	}
	c.Delete("k") // no values yet
	c.Set(nil, "v")
	c.Set("k", nil)
	if _, ok := c.Get("k"); ok {
		t.Fatal("Set with nil key or value should do nothing")
	}
	c.Set("k", "v")
	if v, ok := c.Get("k"); !ok || v != "v" {
		t.Fatalf("Get(k) = %v, %v", v, ok)
	}
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Fatal("Get after Delete should fail")
	}
}

func TestClient_State(t *testing.T) {
	c := &Client{}
	if err := c.CheckState(); err != ErrClientStopped {
		t.Fatalf("CheckState = %v, want ErrClientStopped", err)
	}
	c.SetState(true)
	if err := c.CheckState(); err != nil {
		t.Fatalf("CheckState = %v", err)
	}
	c.reconnecting = true
	if err := c.CheckState(); err != ErrClientReconnecting {
		t.Fatalf("CheckState = %v, want ErrClientReconnecting", err)
	}
}

func TestClient_NewMessage(t *testing.T) {
	c, _, _ := pipeClient(t, NewHandler())
	m1 := c.NewMessage(CmdNotify, "m", "data")
	m2 := c.NewMessage(CmdRequest, "m", "data", map[interface{}]interface{}{"k": "v"})
	if m1.Cmd() != CmdNotify || m1.Method() != "m" || string(m1.Data()) != "data" {
		t.Fatal("unexpected message")
	}
	if m2.Seq() != m1.Seq()+1 {
		t.Fatalf("seq %v then %v, want increasing", m1.Seq(), m2.Seq())
	}
	if v, _ := m2.Get("k"); v != "v" {
		t.Fatal("values not attached")
	}
}

func TestClient_ArgChecks(t *testing.T) {
	c, _, _ := pipeClient(t, NewHandler())
	handler := func(*Context, error) {}
	long := strings.Repeat("m", MaxMethodLen+1)
	ctx := context.Background()

	for name, err := range map[string]error{
		"Call timeout 0":        c.Call("m", nil, nil, 0),
		"Call timeout < 0":      c.Call("m", nil, nil, -1),
		"CallAsync timeout 0":   c.CallAsync("m", nil, handler, 0),
		"CallAsync timeout < 0": c.CallAsync("m", nil, handler, -1),
		"CallAsync nil handler": c.CallAsync("m", nil, nil, time.Second),
		"Notify timeout < 0":    c.Notify("m", nil, -1),
		"Call empty method":     c.Call("", nil, nil, time.Second),
		"Call long method":      c.Call(long, nil, nil, time.Second),
		"CallContext method":    c.CallContext(ctx, "", nil, nil),
		"CallAsync method":      c.CallAsync("", nil, handler, time.Second),
		"Notify method":         c.Notify(long, nil, time.Second),
		"NotifyContext method":  c.NotifyContext(ctx, "", nil),
	} {
		if err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	if err := c.Call("m", nil, nil, 0); err != ErrClientInvalidTimeoutZero {
		t.Fatalf("Call timeout 0 = %v", err)
	}
	if err := c.CallAsync("m", nil, handler, -1); err != ErrClientInvalidTimeoutLessThanZero {
		t.Fatalf("CallAsync timeout < 0 = %v", err)
	}
	if err := c.CallAsync("m", nil, handler, 0); err != ErrClientInvalidTimeoutZero {
		t.Fatalf("CallAsync timeout 0 = %v", err)
	}
	if err := c.CallAsync("m", nil, nil, time.Second); err != ErrClientInvalidAsyncHandler {
		t.Fatalf("CallAsync nil handler = %v", err)
	}
	if err := c.Notify("m", nil, -1); err != ErrClientInvalidTimeoutLessThanZero {
		t.Fatalf("Notify timeout < 0 = %v", err)
	}
}

func TestClient_Stopped(t *testing.T) {
	h := NewHandler()
	var done int32
	h.HandleMessageDone(func(*Client, *Message) { atomic.AddInt32(&done, 1) })
	disconnected := make(chan *Client, 2)
	h.HandleDisconnected(func(c *Client) { disconnected <- c })

	c, _, _ := pipeClient(t, h)
	c.Stop()
	if got := recvWithin(t, disconnected, "OnDisconnected"); got != c {
		t.Fatal("OnDisconnected got another Client")
	}
	c.Stop() // stopping again is fine
	assertNoRecv(t, disconnected, 20*time.Millisecond, "second OnDisconnected")

	ctx := context.Background()
	handler := func(*Context, error) {}
	for name, err := range map[string]error{
		"Call":          c.Call("m", nil, nil, time.Second),
		"CallContext":   c.CallContext(ctx, "m", nil, nil),
		"CallAsync":     c.CallAsync("m", nil, handler, time.Second),
		"Notify":        c.Notify("m", nil, time.Second),
		"NotifyContext": c.NotifyContext(ctx, "m", nil),
		"PushMsg":       c.PushMsg(c.NewMessage(CmdNotify, "m", nil), time.Second),
	} {
		if err != ErrClientStopped {
			t.Fatalf("%s = %v, want ErrClientStopped", name, err)
		}
	}
	// PushMsg hands the message back even when it fails.
	if atomic.LoadInt32(&done) != 1 {
		t.Fatalf("OnMessageDone called %v times, want 1", done)
	}
}

func TestClient_PingPong(t *testing.T) {
	c, _, p := pipeClient(t, NewHandler())

	go c.Ping()
	if m := p.read(); m.Cmd() != CmdPing {
		t.Fatalf("got cmd %v, want ping", m.Cmd())
	}
	go c.Pong()
	if m := p.read(); m.Cmd() != CmdPong {
		t.Fatalf("got cmd %v, want pong", m.Cmd())
	}

	// A ping from the peer is answered with a pong; a pong is ignored.
	p.write(PongMessage.Buffer)
	p.write(PingMessage.Buffer)
	if m := p.read(); m.Cmd() != CmdPong {
		t.Fatalf("got cmd %v, want pong", m.Cmd())
	}
}

func TestClient_Keepalive(t *testing.T) {
	c, _, p := pipeClient(t, NewHandler())
	c.Keepalive(5 * time.Millisecond)
	for i := 0; i < 2; i++ {
		if m := p.read(); m.Cmd() != CmdPing {
			t.Fatalf("got cmd %v, want ping", m.Cmd())
		}
	}
	// The default interval is long; it only needs to be scheduled here.
	c.Keepalive(0)
	c.Stop()
	// No more pings once stopped.
	if _, err := p.tryRead(30 * time.Millisecond); err == nil {
		// One ping may have been in flight when stopping.
		if _, err := p.tryRead(30 * time.Millisecond); err == nil {
			t.Fatal("pings go on after Stop")
		}
	}
	// A stopped Client does not start a keepalive.
	(&Client{}).Keepalive(time.Millisecond)
}

// fullQueueClient returns a Client whose send queue of size 1 is full: the
// send loop is blocked writing a first message, since the peer does not read,
// and a second one waits in the queue.
func fullQueueClient(t *testing.T, h Handler) (*Client, *peer) {
	h.SetSendQueueSize(1)
	c, _, p := pipeClient(t, h)
	for i := 0; i < 2; i++ {
		if err := c.Notify("m", "x", time.Second); err != nil {
			t.Fatalf("Notify: %v", err)
		}
	}
	waitFor(t, "queue full", func() bool { return len(c.chSend) == 1 })
	return c, p
}

func TestClient_Overstock(t *testing.T) {
	h := NewHandler()
	var overstocked, done int32
	h.HandleOverstock(func(c *Client, m *Message) { atomic.AddInt32(&overstocked, 1) })
	h.HandleMessageDone(func(c *Client, m *Message) { atomic.AddInt32(&done, 1) })
	c, _ := fullQueueClient(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Not waiting.
	if err := c.Notify("m", "x", TimeZero); err != ErrClientOverstock {
		t.Fatalf("Notify(TimeZero) = %v", err)
	}
	if err := c.PushMsg(c.NewMessage(CmdNotify, "m", "x"), TimeZero); err != ErrClientOverstock {
		t.Fatalf("PushMsg(TimeZero) = %v", err)
	}
	if n := atomic.LoadInt32(&overstocked); n != 2 {
		t.Fatalf("OnOverstock called %v times, want 2", n)
	}
	// OnOverstock is followed by OnMessageDone.
	if n := atomic.LoadInt32(&done); n != 2 {
		t.Fatalf("OnMessageDone called %v times, want 2", n)
	}

	// Waiting up to a timeout.
	for name, err := range map[string]error{
		"Notify":        c.Notify("m", "x", 10*time.Millisecond),
		"PushMsg":       c.PushMsg(c.NewMessage(CmdNotify, "m", "x"), 10*time.Millisecond),
		"Call":          c.Call("m", "x", nil, 10*time.Millisecond),
		"CallContext":   c.CallContext(ctx, "m", "x", nil),
		"NotifyContext": c.NotifyContext(ctx, "m", "x"),
		"CallAsync":     c.CallAsync("m", "x", func(*Context, error) {}, 10*time.Millisecond),
	} {
		if err != ErrClientTimeout {
			t.Fatalf("%s = %v, want ErrClientTimeout", name, err)
		}
	}
}

func TestClient_StopWhileQueued(t *testing.T) {
	c, _ := fullQueueClient(t, NewHandler())

	// Callers blocked on the full queue return once the Client stops. As the
	// send loop drains the queue when stopping, a push may still get in.
	pushes := map[string]func() error{
		"PushMsg(TimeForever)": func() error { return c.PushMsg(c.NewMessage(CmdNotify, "m", "x"), TimeForever) },
		"PushMsg(<0)":          func() error { return c.PushMsg(c.NewMessage(CmdNotify, "m", "x"), -1) },
		"Notify":               func() error { return c.Notify("m", "x", time.Minute) },
		"NotifyContext":        func() error { return c.NotifyContext(context.Background(), "m", "x") },
		"CallAsync":            func() error { return c.CallAsync("m", "x", func(*Context, error) {}, time.Minute) },
	}
	// Calls also wait for the response, so they always fail.
	calls := map[string]func() error{
		"Call":        func() error { return c.Call("m", "x", nil, time.Minute) },
		"CallContext": func() error { return c.CallContext(context.Background(), "m", "x", nil) },
	}
	pushErrs := make(chan error, len(pushes))
	for _, push := range pushes {
		go func(push func() error) { pushErrs <- push() }(push)
	}
	callErrs := make(chan error, len(calls))
	for _, call := range calls {
		go func(call func() error) { callErrs <- call() }(call)
	}
	time.Sleep(20 * time.Millisecond)
	c.Stop()
	for range pushes {
		if err := recvWithin(t, pushErrs, "blocked push"); err != nil && err != ErrClientStopped {
			t.Fatalf("blocked push = %v, want ErrClientStopped or nil", err)
		}
	}
	for range calls {
		if err := recvWithin(t, callErrs, "blocked call"); err != ErrClientStopped {
			t.Fatalf("blocked call = %v, want ErrClientStopped", err)
		}
	}
}

func TestClient_PendingCallStopped(t *testing.T) {
	// A Call sent but not answered yet returns once the Client stops.
	c, _, p := pipeClient(t, NewHandler())
	errs := make(chan error, 1)
	go func() { errs <- c.Call("m", "x", nil, time.Minute) }()
	p.read()
	c.Stop()
	if err := recvWithin(t, errs, "Call"); err != ErrClientStopped && err != ErrClientReconnecting {
		t.Fatalf("Call = %v", err)
	}
}

func TestClient_WriteErrors(t *testing.T) {
	t.Run("Sync", func(t *testing.T) {
		h := NewHandler()
		h.SetAsyncWrite(false)
		c, conn, _ := pipeClient(t, h)
		conn.setFailWrite(true)
		if err := c.Notify("m", "x", 0); err != errTestWrite {
			t.Fatalf("Notify = %v", err)
		}
		// The conn is closed on a write error.
		waitFor(t, "client stopped", func() bool { return !clientRunning(c) })
	})

	for _, mode := range []config{
		{"Async", func(h Handler) {}},
		{"AsyncNoBatch", func(h Handler) { h.SetBatchSend(false) }},
		{"AsyncBatchBuffer", func(h Handler) { h.SetSendBufferSize(1024) }},
		{"Writev", func(h Handler) { h.SetAsyncWritev(true) }},
	} {
		mode := mode
		t.Run(mode.name, func(t *testing.T) {
			h := NewHandler()
			mode.setup(h)
			var done int32
			h.HandleMessageDone(func(*Client, *Message) { atomic.AddInt32(&done, 1) })
			c, conn, _ := pipeClient(t, h)
			conn.setFailWrite(true)
			if err := c.Notify("m", "x", time.Second); err != nil {
				t.Fatalf("Notify = %v", err)
			}
			waitFor(t, "client stopped", func() bool { return !clientRunning(c) })
			waitFor(t, "message done", func() bool { return atomic.LoadInt32(&done) == 1 })
		})
	}
}

func TestClient_DropWhileReconnecting(t *testing.T) {
	// Messages that reach the writers while reconnecting are dropped. The
	// public APIs check the state first, so drive the writers directly.
	newHandler := func(setup func(h Handler)) (Handler, *int32) {
		h := NewHandler()
		setup(h)
		dropped := new(int32)
		h.HandleMessageDropped(func(*Client, *Message) { atomic.AddInt32(dropped, 1) })
		return h, dropped
	}

	t.Run("Sync", func(t *testing.T) {
		h, dropped := newHandler(func(h Handler) { h.SetAsyncWrite(false) })
		c, _, _ := pipeClient(t, h)
		c.reconnecting = true
		msg := c.newRequestMessage(CmdRequest, "m", nil, false, false)
		sess := newSession(msg.Seq())
		c.addSession(msg.Seq(), sess)
		if err := c.writeSync(msg); err != ErrClientReconnecting {
			t.Fatalf("writeSync = %v", err)
		}
		// The pending call of a dropped request fails at once.
		if _, ok := <-sess.done; ok {
			t.Fatal("session should be closed")
		}
		if err := c.writeSync(c.newRequestMessage(CmdRequest, "m", nil, false, true)); err != ErrClientReconnecting {
			t.Fatalf("writeSync async = %v", err)
		}
		if *dropped != 2 {
			t.Fatalf("dropped %v, want 2", *dropped)
		}
		c.reconnecting = false
	})

	t.Run("Writev", func(t *testing.T) {
		h, dropped := newHandler(func(h Handler) { h.SetAsyncWritev(true) })
		c, _, _ := pipeClient(t, h)
		c.reconnecting = true
		if err := c.pushWritev(c.newRequestMessage(CmdNotify, "m", nil, false, true)); err != ErrClientReconnecting {
			t.Fatalf("pushWritev = %v", err)
		}
		if *dropped != 1 {
			t.Fatalf("dropped %v, want 1", *dropped)
		}
		c.reconnecting = false
	})

	for _, mode := range []config{
		{"Batch", func(h Handler) {}},
		{"NoBatch", func(h Handler) { h.SetBatchSend(false) }},
	} {
		mode := mode
		t.Run(mode.name, func(t *testing.T) {
			h, dropped := newHandler(mode.setup)
			c, _, _ := pipeClient(t, h)
			c.reconnecting = true
			c.chSend <- c.newRequestMessage(CmdNotify, "m", nil, false, true)
			waitFor(t, "dropped", func() bool { return atomic.LoadInt32(dropped) == 1 })
			c.Stop()
		})
	}
}

func TestClient_WritevDropAfterWriteError(t *testing.T) {
	// Messages queued behind a failed writev are dropped, not written.
	h := NewHandler()
	h.SetAsyncWritev(true)
	var dropped, done int32
	h.HandleMessageDropped(func(*Client, *Message) { atomic.AddInt32(&dropped, 1) })
	h.HandleMessageDone(func(*Client, *Message) { atomic.AddInt32(&done, 1) })
	c, _, p := pipeClient(t, h)

	// The first write blocks as the peer does not read; queue more behind it.
	if err := c.Notify("m", "first", 0); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	for i := 0; i < 3; i++ {
		if err := c.Notify("m", "queued", 0); err != nil {
			t.Fatalf("Notify: %v", err)
		}
	}
	p.conn.Close()
	waitFor(t, "queued messages dropped", func() bool { return atomic.LoadInt32(&dropped) == 3 })
	// Each dropped message is also done, besides the failed first one.
	waitFor(t, "messages done", func() bool { return atomic.LoadInt32(&done) == 4 })

	// Pushing after the Client stopped fails.
	waitFor(t, "client stopped", func() bool { return !clientRunning(c) })
	if err := c.pushWritev(c.newRequestMessage(CmdNotify, "m", nil, false, true)); err != ErrClientStopped {
		t.Fatalf("pushWritev after stop = %v", err)
	}
}

func TestClient_ResponseParsing(t *testing.T) {
	c, _, _ := pipeClient(t, NewHandler())

	if err := c.parseResponse(nil, nil); err != ErrClientReconnecting {
		t.Fatalf("parseResponse(nil) = %v", err)
	}
	notRsp := newMessage(CmdRequest, "m", "x", false, false, 1, nil, nil, nil)
	if err := c.parseResponse(notRsp, nil); err != ErrInvalidRspMessage {
		t.Fatalf("parseResponse(request) = %v", err)
	}
	if _, err := c.responseData(nil); err != ErrClientReconnecting {
		t.Fatalf("responseData(nil) = %v", err)
	}
	if _, err := c.responseData(notRsp); err != ErrInvalidRspMessage {
		t.Fatalf("responseData(request) = %v", err)
	}
	errRsp := newMessage(CmdResponse, "m", "bad", true, false, 1, nil, nil, nil)
	if _, err := c.responseData(errRsp); err == nil || err.Error() != "bad" {
		t.Fatalf("responseData(error) = %v", err)
	}

	rsp := newMessage(CmdResponse, "m", &payload{A: 1}, false, false, 1, nil, nil, nil)
	data, err := c.responseData(rsp)
	if err != nil {
		t.Fatalf("responseData = %v", err)
	}
	var (
		s string
		b []byte
		p payload
	)
	if err := c.parseData(data, nil); err != nil {
		t.Fatalf("parseData(nil) = %v", err)
	}
	if c.parseData(data, &s); s != `{"A":1,"B":""}` {
		t.Fatalf("parseData(string) = %q", s)
	}
	if c.parseData(data, &b); string(b) != s {
		t.Fatalf("parseData(bytes) = %q", b)
	}
	if err := c.parseData(data, &p); err != nil || p.A != 1 {
		t.Fatalf("parseData(struct) = %v, %+v", err, p)
	}
}

func TestClient_Restart(t *testing.T) {
	_, addr := startServer(t, func(h Handler) {
		h.Handle(routeEcho, func(ctx *Context) { ctx.Write(ctx.Body()) })
	})
	for _, mode := range writeModes {
		mode := mode
		t.Run(mode.name, func(t *testing.T) {
			var dialFail int32
			dialer := func() (net.Conn, error) {
				if atomic.LoadInt32(&dialFail) == 1 {
					return nil, errors.New("dial failed")
				}
				return tcpDialer(addr)()
			}
			h := NewHandler()
			mode.setup(h)
			var disconnected int32
			h.HandleDisconnected(func(*Client) { atomic.AddInt32(&disconnected, 1) })
			c, err := NewClient(dialer, h)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			defer c.Stop()

			for i := 1; i <= 3; i++ {
				c.Set("k", "v")
				old := c.Conn
				if err := c.Restart(); err != nil {
					t.Fatalf("Restart: %v", err)
				}
				if c.Conn == old {
					t.Fatal("Restart should dial a new conn")
				}
				if _, ok := c.Get("k"); ok {
					t.Fatal("Restart should clear the values")
				}
				// The old generation stopped exactly once per Restart.
				waitFor(t, "OnDisconnected", func() bool { return atomic.LoadInt32(&disconnected) == int32(i) })
				var rsp string
				if err := c.Call(routeEcho, "after restart", &rsp, time.Second); err != nil || rsp != "after restart" {
					t.Fatalf("Call after Restart = %v, %q", err, rsp)
				}
			}

			atomic.StoreInt32(&dialFail, 1)
			if err := c.Restart(); err == nil {
				t.Fatal("Restart with a failing dialer should fail")
			}
			if err := c.CheckState(); err != ErrClientStopped {
				t.Fatalf("CheckState after failed Restart = %v", err)
			}
		})
	}
}

func TestClient_Reconnect(t *testing.T) {
	dialer, peers := pipeDialer(t)
	h := NewHandler()
	h.HandleStream(routeStream, func(*Stream) {})
	infos := make(chan *ReconnectInfo, 4)
	connected := make(chan *Client, 4)
	h.HandleReconnect(func(c *Client, info *ReconnectInfo) { infos <- info })
	h.HandleConnected(func(c *Client) { connected <- c })
	c, err := NewClient(dialer, h)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Stop()
	p1 := recvWithin(t, peers, "first conn")
	recvWithin(t, connected, "OnConnected")

	// Leave a Call, a CallAsync and a Stream pending.
	callErr := make(chan error, 1)
	go func() { callErr <- c.Call("m", "x", nil, time.Minute) }()
	p1.read()
	asyncErr := make(chan error, 1)
	if err := c.CallAsync("m", "x", func(ctx *Context, err error) { asyncErr <- err }, time.Minute); err != nil {
		t.Fatalf("CallAsync: %v", err)
	}
	p1.read()
	stream := c.NewStream(routeStream)

	// Breaking the conn fails them all, and the Client reconnects at once.
	p1.conn.Close()
	if err := recvWithin(t, callErr, "Call"); err != ErrClientReconnecting {
		t.Fatalf("pending Call = %v", err)
	}
	if err := recvWithin(t, asyncErr, "CallAsync"); err != ErrClientReconnecting {
		t.Fatalf("pending CallAsync = %v", err)
	}
	var s string
	if err := stream.Recv(&s); err == nil {
		t.Fatal("pending Stream Recv should fail")
	}

	p2 := recvWithin(t, peers, "second conn")
	info := recvWithin(t, infos, "OnReconnect")
	if !info.Success || info.Err != nil || info.Times != 1 || info.MaxTimes != 0 || info.Addr == "" {
		t.Fatalf("unexpected info %+v", info)
	}
	recvWithin(t, connected, "OnConnected after reconnect")

	// The Client works on the new conn.
	// The Stream's EOF may be sent on the new conn first.
	go c.Notify("m", "after", time.Second)
	m := p2.read()
	if m.Cmd() == CmdStream {
		m = p2.read()
	}
	if m.Cmd() != CmdNotify || string(m.Data()) != "after" {
		t.Fatalf("got cmd %v, %q", m.Cmd(), m.Data())
	}
}

func TestClient_ReconnectAttempts(t *testing.T) {
	// Failed attempts are 1 second apart; run these in parallel.
	errDial := errors.New("dial failed")

	t.Run("RecoverAfterFailure", func(t *testing.T) {
		t.Parallel()
		dialer, peers := pipeDialer(t)
		var dials int32
		h := NewHandler()
		infos := make(chan *ReconnectInfo, 4)
		h.HandleReconnect(func(c *Client, info *ReconnectInfo) { infos <- info })
		c, err := NewClient(func() (net.Conn, error) {
			if atomic.AddInt32(&dials, 1) == 2 {
				return nil, errDial
			}
			return dialer()
		}, h)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer c.Stop()
		recvWithin(t, peers, "first conn").conn.Close()

		if info := recvWithin(t, infos, "failed attempt"); info.Success || info.Err != errDial || info.Times != 1 {
			t.Fatalf("unexpected info %+v", info)
		}
		if info := recvWithin(t, infos, "second attempt"); !info.Success || info.Times != 2 {
			t.Fatalf("unexpected info %+v", info)
		}
	})

	t.Run("GiveUp", func(t *testing.T) {
		t.Parallel()
		dialer, peers := pipeDialer(t)
		var dials int32
		h := NewHandler()
		h.SetMaxReconnectTimes(1)
		disconnected := make(chan *Client, 1)
		h.HandleDisconnected(func(c *Client) { disconnected <- c })
		c, err := NewClient(func() (net.Conn, error) {
			if atomic.AddInt32(&dials, 1) > 1 {
				return nil, errDial
			}
			return dialer()
		}, h)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		recvWithin(t, peers, "first conn").conn.Close()

		recvWithin(t, disconnected, "OnDisconnected")
		if err := c.CheckState(); err != ErrClientStopped {
			t.Fatalf("CheckState = %v", err)
		}
		if n := atomic.LoadInt32(&dials); n != 2 {
			t.Fatalf("dialed %v times, want 2", n)
		}
	})

	t.Run("StopWhileReconnecting", func(t *testing.T) {
		t.Parallel()
		dialer, peers := pipeDialer(t)
		var dials int32
		h := NewHandler()
		infos := make(chan *ReconnectInfo, 8)
		h.HandleReconnect(func(c *Client, info *ReconnectInfo) { infos <- info })
		disconnected := make(chan *Client, 1)
		h.HandleDisconnected(func(c *Client) { disconnected <- c })
		c, err := NewClient(func() (net.Conn, error) {
			if atomic.AddInt32(&dials, 1) > 1 {
				return nil, errDial
			}
			return dialer()
		}, h)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		recvWithin(t, peers, "first conn").conn.Close()
		recvWithin(t, infos, "failed attempt")
		c.Stop()
		recvWithin(t, disconnected, "OnDisconnected")
		if n := atomic.LoadInt32(&dials); n != 2 {
			t.Fatalf("dialed %v times, want 2", n)
		}
	})
}

func TestNewClient(t *testing.T) {
	errDial := errors.New("dial failed")
	if _, err := NewClient(func() (net.Conn, error) { return nil, errDial }); err != errDial {
		t.Fatalf("NewClient = %v", err)
	}

	dialer, peers := pipeDialer(t)
	// A non-Handler arg is ignored, and a clone of DefaultHandler is used.
	c, err := NewClient(dialer, "not a handler")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Stop()
	recvWithin(t, peers, "conn")
	if c.Handler == nil || c.Handler == DefaultHandler || c.Codec == nil || !c.IsClient() {
		t.Fatal("unexpected Client setup")
	}
}

func TestClientPool(t *testing.T) {
	_, addr := startServer(t, func(h Handler) {
		h.Handle(routeEcho, func(ctx *Context) { ctx.Write(ctx.Body()) })
	})
	h := NewHandler()
	pool, err := NewClientPool(tcpDialer(addr), 3, h)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}
	defer pool.Stop()

	if pool.Size() != 3 {
		t.Fatalf("Size = %v", pool.Size())
	}
	if pool.Get(0) != pool.Get(3) || pool.Get(0) == pool.Get(1) {
		t.Fatal("Get should index modulo the size")
	}
	if pool.Handler() != h {
		t.Fatal("the Clients should share the Handler")
	}
	// Next goes round robin.
	seen := map[*Client]bool{}
	for i := 0; i < 3; i++ {
		c := pool.Next()
		seen[c] = true
		var rsp string
		if err := c.Call(routeEcho, "pool", &rsp, time.Second); err != nil || rsp != "pool" {
			t.Fatalf("Call = %v, %q", err, rsp)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("Next returned %v distinct Clients, want 3", len(seen))
	}

	// Next skips stopped Clients.
	pool.Get(0).Stop()
	pool.Get(1).Stop()
	for i := 0; i < 3; i++ {
		if pool.Next() != pool.Get(2) {
			t.Fatal("Next should skip stopped Clients")
		}
	}
	// With all stopped, it still returns one.
	pool.Get(2).Stop()
	if pool.Next() == nil {
		t.Fatal("Next should return a Client")
	}
}

func TestClientPool_Errors(t *testing.T) {
	_, addr := startServer(t, nil)
	errDial := errors.New("dial failed")
	var (
		mu      sync.Mutex
		created []*Client
	)
	h := NewHandler()
	h.HandleConnected(func(c *Client) {
		mu.Lock()
		created = append(created, c)
		mu.Unlock()
	})

	failSecond := func() DialerFunc {
		var n int32
		return func() (net.Conn, error) {
			if atomic.AddInt32(&n, 1) == 2 {
				return nil, errDial
			}
			return tcpDialer(addr)()
		}
	}

	if _, err := NewClientPool(failSecond(), 3, h); err != errDial {
		t.Fatalf("NewClientPool = %v", err)
	}
	if _, err := NewClientPoolFromDialers(nil); err != ErrClientInvalidPoolDialers {
		t.Fatalf("NewClientPoolFromDialers(nil) = %v", err)
	}
	d := failSecond()
	if _, err := NewClientPoolFromDialers([]DialerFunc{d, d, d}, h); err != errDial {
		t.Fatalf("NewClientPoolFromDialers = %v", err)
	}
	// The Clients created before the failure are stopped.
	waitFor(t, "created clients stopped", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range created {
			if clientRunning(c) {
				return false
			}
		}
		return len(created) == 2
	})

	// Without a Handler arg, a clone of DefaultHandler is used.
	pool, err := NewClientPool(tcpDialer(addr), 1)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}
	pool.Stop()
	pool, err = NewClientPoolFromDialers([]DialerFunc{tcpDialer(addr), tcpDialer(addr)})
	if err != nil || pool.Size() != 2 {
		t.Fatalf("NewClientPoolFromDialers = %v", err)
	}
	pool.Stop()
}

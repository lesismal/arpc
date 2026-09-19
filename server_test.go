// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func serverClients(s *Server) int {
	n := 0
	s.ForEach(func(*Client) { n++ })
	return n
}

func TestNewServer(t *testing.T) {
	s := NewServer()
	if s.Handler == nil || s.Handler == DefaultHandler || s.Codec == nil || s.Handler.LogTag() != "[ARPC SVR]" {
		t.Fatal("unexpected Server setup")
	}
	m1 := s.NewMessage(CmdNotify, "m", "x")
	m2 := s.NewMessage(CmdNotify, "m", "x", map[interface{}]interface{}{"k": "v"})
	if m2.Seq() != m1.Seq()+1 || m1.handler != s.Handler {
		t.Fatal("unexpected messages")
	}
	if v, _ := m2.Get("k"); v != "v" {
		t.Fatal("values not attached")
	}
}

func TestServer_Clients(t *testing.T) {
	disconnected := make(chan *Client, 4)
	s, addr := startServer(t, func(h Handler) {
		h.HandleDisconnected(func(c *Client) { disconnected <- c })
	})
	const n = 3
	clients := make([]*Client, n)
	for i := range clients {
		clients[i] = dialClient(t, addr, nil)
	}
	waitFor(t, "clients accepted", func() bool { return serverClients(s) == n })
	if s.Accepted != n || atomic.LoadInt64(&s.CurrLoad) != n {
		t.Fatalf("Accepted = %v, CurrLoad = %v", s.Accepted, s.CurrLoad)
	}

	// A Client leaving is removed from the Server.
	clients[0].Stop()
	recvWithin(t, disconnected, "OnDisconnected")
	waitFor(t, "client removed", func() bool { return serverClients(s) == n-1 && atomic.LoadInt64(&s.CurrLoad) == n-1 })
}

func TestServer_Broadcast(t *testing.T) {
	s, addr := startServer(t, func(h Handler) { h.EnablePool(true) })
	got := make(chan string, 16)
	for i := 0; i < 3; i++ {
		dialClient(t, addr, func(h Handler) {
			h.Handle("/b", func(ctx *Context) {
				var s string
				ctx.Bind(&s)
				got <- s
			})
		})
	}
	waitFor(t, "clients accepted", func() bool { return serverClients(s) == 3 })
	// Mark the server-side Clients to filter on.
	marked := 0
	s.ForEach(func(c *Client) {
		if marked == 0 {
			c.Set("mark", true)
			marked++
		}
	})

	s.Broadcast("/b", "all")
	for i := 0; i < 3; i++ {
		if v := recvWithin(t, got, "broadcast"); v != "all" {
			t.Fatalf("got %q", v)
		}
	}

	isMarked := func(c *Client) bool { _, ok := c.Get("mark"); return ok }
	s.BroadcastWithFilter("/b", "marked", isMarked)
	if v := recvWithin(t, got, "filtered broadcast"); v != "marked" {
		t.Fatalf("got %q", v)
	}
	assertNoRecv(t, got, 20*time.Millisecond, "broadcast to unmarked")

	s.BroadcastWithFilter("/b", "nil filter", nil, map[interface{}]interface{}{"k": "v"})
	for i := 0; i < 3; i++ {
		if v := recvWithin(t, got, "nil filter broadcast"); v != "nil filter" {
			t.Fatalf("got %q", v)
		}
	}

	var count int
	s.ForEachWithFilter(func(*Client) { count++ }, isMarked)
	s.ForEachWithFilter(func(*Client) { count++ }, nil)
	if count != 4 {
		t.Fatalf("ForEachWithFilter visited %v", count)
	}
}

func TestServer_MaxLoad(t *testing.T) {
	s := NewServer()
	s.MaxLoad = 1
	addr := serve(t, s)
	dialClient(t, addr, nil)
	waitFor(t, "client accepted", func() bool { return serverClients(s) == 1 })

	// The next conn is closed at once.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(waitTimeout))
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("Read = %v, want io.EOF", err)
	}
	// The load is taken back right after the close, which the Read may beat.
	waitFor(t, "load taken back", func() bool { return atomic.LoadInt64(&s.CurrLoad) == 1 })
	if s.Accepted != 1 {
		t.Fatalf("Accepted = %v", s.Accepted)
	}
}

func TestServer_KickInHandler(t *testing.T) {
	// A handler on the read goroutine stopping its Client must still remove
	// it from the Server and call OnDisconnected.
	disconnected := make(chan *Client, 1)
	s, addr := startServer(t, func(h Handler) {
		h.Handle("/kick", func(ctx *Context) { ctx.Client.Stop() }, false)
		h.HandleDisconnected(func(c *Client) { disconnected <- c })
	})
	// A raw conn, so that nothing reconnects.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	waitFor(t, "client accepted", func() bool { return serverClients(s) == 1 })
	conn.Write(newMessage(CmdNotify, "/kick", nil, false, false, 1, nil, nil, nil).Buffer)
	recvWithin(t, disconnected, "OnDisconnected")
	waitFor(t, "client removed", func() bool { return serverClients(s) == 0 && atomic.LoadInt64(&s.CurrLoad) == 0 })
}

func TestServer_StopAndShutdown(t *testing.T) {
	t.Run("Stop", func(t *testing.T) {
		s := NewServer()
		disconnected := make(chan *Client, 1)
		s.Handler.HandleDisconnected(func(c *Client) { disconnected <- c })
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		served := make(chan error, 1)
		go func() { served <- s.Serve(ln) }()
		waitFor(t, "server running", s.isRunning)
		dialClient(t, ln.Addr().String(), func(h Handler) { h.SetMaxReconnectTimes(1) })
		waitFor(t, "client accepted", func() bool { return serverClients(s) == 1 })

		if err := s.Stop(); err != nil {
			t.Fatalf("Stop = %v", err)
		}
		// Serve returns once stopped. Its error is the last Accept result: the
		// closed listener's error, or nil if Stop came right after an accept.
		recvWithin(t, served, "Serve")
		// All conns are stopped.
		recvWithin(t, disconnected, "OnDisconnected")
		if serverClients(s) != 0 {
			t.Fatal("clients not cleared")
		}
	})

	t.Run("Shutdown", func(t *testing.T) {
		s := NewServer()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go s.Serve(ln)
		waitFor(t, "server running", s.isRunning)
		if err := s.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown = %v", err)
		}
	})

	t.Run("ShutdownTimeout", func(t *testing.T) {
		// The accept loop does not exit while Accept keeps blocking.
		ln := newFakeListener()
		s := NewServer()
		go s.Serve(ln)
		waitFor(t, "server running", s.isRunning)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		ln.ignoreClose = true
		if err := s.Shutdown(ctx); err != ErrTimeout {
			t.Fatalf("Shutdown = %v, want ErrTimeout", err)
		}
		ln.fail(errors.New("closed"))
	})
}

func TestServer_Run(t *testing.T) {
	s := NewServer()
	if err := s.Run("256.0.0.1:-1"); err == nil {
		t.Fatal("Run on a bad address should fail")
	}

	s = NewServer()
	s.Handler.Handle(routeEcho, func(ctx *Context) { ctx.Write(ctx.Body()) })
	served := make(chan error, 1)
	go func() { served <- s.Run("127.0.0.1:0") }()
	waitFor(t, "server running", s.isRunning)
	s.mux.Lock()
	addr := s.Listener.Addr().String()
	s.mux.Unlock()

	c := dialClient(t, addr, func(h Handler) { h.SetMaxReconnectTimes(1) })
	var rsp string
	if err := c.Call(routeEcho, "run", &rsp, time.Second); err != nil || rsp != "run" {
		t.Fatalf("Call = %v, %q", err, rsp)
	}
	c.Stop()
	s.Stop()
	recvWithin(t, served, "Run")
}

// fakeListener is a net.Listener whose Accept results are fed by the test.
type fakeListener struct {
	ch          chan error
	ignoreClose bool
}

func newFakeListener() *fakeListener {
	return &fakeListener{ch: make(chan error, 4)}
}

func (l *fakeListener) fail(err error) { l.ch <- err }

func (l *fakeListener) Accept() (net.Conn, error) { return nil, <-l.ch }

func (l *fakeListener) Close() error {
	if !l.ignoreClose {
		l.fail(errors.New("closed"))
	}
	return nil
}

func (l *fakeListener) Addr() net.Addr { return &net.TCPAddr{} }

// temporaryError is an Accept error the Server retries.
type temporaryError struct{}

func (temporaryError) Error() string   { return "temporary" }
func (temporaryError) Timeout() bool   { return false }
func (temporaryError) Temporary() bool { return true }

func TestServer_AcceptErrors(t *testing.T) {
	ln := newFakeListener()
	s := NewServer()
	served := make(chan error, 1)
	go func() { served <- s.Serve(ln) }()

	// A temporary error is retried; another one ends the loop.
	errFatal := errors.New("fatal")
	ln.fail(temporaryError{})
	ln.fail(errFatal)
	if err := recvWithin(t, served, "Serve"); err != errFatal {
		t.Fatalf("Serve = %v, want the fatal error", err)
	}
}

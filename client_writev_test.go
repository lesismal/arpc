// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const writevServerAddr = "localhost:11001"

// newWritevHandler returns a coder-free handler with the writev async-send path
// enabled, so newRequestMessage keeps the data apart from the header.
func newWritevHandler() Handler {
	h := NewHandler()
	h.SetAsyncWritev(true)
	return h
}

var (
	writevMethodEcho   = "/writev/echo"
	writevMethodStruct = "/writev/struct"
	writevMethodBytes  = "/writev/bytes"
	writevMethodNotify = "/writev/notify"
)

// writevNotifyCh receives payloads observed by the notify handler for the
// []byte-mutation safety test.
var writevNotifyCh = make(chan string, 1024)

var (
	writevServerOnce sync.Once
	writevServer     *Server
)

// initWritevServer starts a single shared writev server for all writev tests.
// It is never stopped during the run, mirroring the existing suite's shared
// server pattern and avoiding the pre-existing Server.running data race that a
// per-test Stop() would trigger.
func initWritevServer(t *testing.T) *Server {
	writevServerOnce.Do(func() {
		s := NewServer()
		s.Handler = newWritevHandler()
		s.Handler.Handle(writevMethodEcho, func(ctx *Context) {
			var s string
			ctx.Bind(&s)
			ctx.Write(s)
		}, true)
		s.Handler.Handle(writevMethodStruct, func(ctx *Context) {
			var v MessageTest
			ctx.Bind(&v)
			ctx.Write(&v)
		}, true)
		s.Handler.Handle(writevMethodBytes, func(ctx *Context) {
			var b []byte
			ctx.Bind(&b)
			ctx.Write(b)
		}, true)
		s.Handler.Handle(writevMethodNotify, func(ctx *Context) {
			var b []byte
			ctx.Bind(&b)
			writevNotifyCh <- string(b)
		}, false)

		ln, err := net.Listen("tcp", writevServerAddr)
		if err != nil {
			t.Fatalf("listen failed: %v", err)
		}
		go s.Serve(ln)
		writevServer = s
		time.Sleep(time.Second / 10)
	})
	return writevServer
}

func writevDialer() (net.Conn, error) {
	return net.DialTimeout("tcp", writevServerAddr, time.Second)
}

func newWritevClient(t *testing.T) *Client {
	c, err := NewClient(writevDialer, newWritevHandler())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	return c
}

func TestWritev_Call(t *testing.T) {
	initWritevServer(t)

	c := newWritevClient(t)
	defer c.Stop()

	// string echo (split path: head + marshaled body)
	for i := 0; i < 50; i++ {
		req := fmt.Sprintf("hello-%d", i)
		var rsp string
		if err := c.Call(writevMethodEcho, req, &rsp, time.Second); err != nil {
			t.Fatalf("Call echo failed: %v", err)
		}
		if rsp != req {
			t.Fatalf("Call echo mismatch: got %q want %q", rsp, req)
		}
	}

	// struct round-trip
	for i := 0; i < 50; i++ {
		req := MessageTest{A: i, B: fmt.Sprintf("v-%d", i)}
		var rsp MessageTest
		if err := c.Call(writevMethodStruct, &req, &rsp, time.Second); err != nil {
			t.Fatalf("Call struct failed: %v", err)
		}
		if rsp != req {
			t.Fatalf("Call struct mismatch: got %+v want %+v", rsp, req)
		}
	}

	// []byte round-trip
	for i := 0; i < 50; i++ {
		req := []byte(fmt.Sprintf("bytes-%d", i))
		var rsp []byte
		if err := c.Call(writevMethodBytes, req, &rsp, time.Second); err != nil {
			t.Fatalf("Call bytes failed: %v", err)
		}
		if !bytes.Equal(rsp, req) {
			t.Fatalf("Call bytes mismatch: got %q want %q", rsp, req)
		}
	}
}

func TestWritev_CallAsync(t *testing.T) {
	initWritevServer(t)

	c := newWritevClient(t)
	defer c.Stop()

	var wg sync.WaitGroup
	var got int32
	n := 100
	for i := 0; i < n; i++ {
		req := fmt.Sprintf("async-%d", i)
		wg.Add(1)
		err := c.CallAsync(writevMethodEcho, req, func(ctx *Context, err error) {
			defer wg.Done()
			if err != nil {
				return
			}
			var rsp string
			ctx.Bind(&rsp)
			if rsp == req {
				atomic.AddInt32(&got, 1)
			}
		}, time.Second)
		if err != nil {
			wg.Done()
			t.Fatalf("CallAsync failed: %v", err)
		}
	}
	wg.Wait()
	if int(got) != n {
		t.Fatalf("CallAsync mismatch: got %d want %d", got, n)
	}
}

// TestWritev_NotifyBytesSafety verifies that mutating a []byte argument right
// after Notify returns does not corrupt the bytes actually sent — the writev
// path must copy caller-mutable slices into a pooled buffer.
func TestWritev_NotifyBytesSafety(t *testing.T) {
	initWritevServer(t)

	c := newWritevClient(t)
	defer c.Stop()

	// drain any residue
	for len(writevNotifyCh) > 0 {
		<-writevNotifyCh
	}

	n := 200
	want := map[string]bool{}
	for i := 0; i < n; i++ {
		original := fmt.Sprintf("original-payload-%06d", i)
		payload := []byte(original)
		want[original] = true
		if err := c.Notify(writevMethodNotify, payload, time.Second); err != nil {
			t.Fatalf("Notify failed: %v", err)
		}
		// Immediately clobber the caller slice; a correct implementation has
		// already copied it, so the server must still observe `original`.
		for j := range payload {
			payload[j] = 'X'
		}
	}

	timeout := time.After(5 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case got := <-writevNotifyCh:
			if !want[got] {
				t.Fatalf("notify payload corrupted/unexpected: %q", got)
			}
			delete(want, got)
		case <-timeout:
			t.Fatalf("timed out waiting for notifies, %d received", i)
		}
	}
}

func TestWritev_Concurrent(t *testing.T) {
	initWritevServer(t)

	c := newWritevClient(t)
	defer c.Stop()

	var wg sync.WaitGroup
	goroutines := 16
	perG := 50
	var fail int32
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				req := fmt.Sprintf("g%d-i%d", g, i)
				var rsp string
				if err := c.Call(writevMethodEcho, req, &rsp, 3*time.Second); err != nil || rsp != req {
					atomic.AddInt32(&fail, 1)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if fail != 0 {
		t.Fatalf("concurrent writev calls failed: %d goroutines had errors", fail)
	}
}

// TestWritev_Broadcast exercises the server-side push path (shared contiguous
// *Message enqueued as a single writev entry) under the writev writer.
func TestWritev_Broadcast(t *testing.T) {
	s := initWritevServer(t)

	recvCh := make(chan string, 16)
	ch := newWritevHandler()
	ch.Handle(writevMethodNotify, func(ctx *Context) {
		var b []byte
		ctx.Bind(&b)
		recvCh <- string(b)
	}, false)

	clients := make([]*Client, 3)
	for i := range clients {
		c, err := NewClient(writevDialer, ch)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		clients[i] = c
		defer c.Stop()
	}
	time.Sleep(time.Second / 5)

	s.Broadcast(writevMethodNotify, []byte("broadcast-msg"))

	timeout := time.After(3 * time.Second)
	for i := 0; i < len(clients); i++ {
		select {
		case got := <-recvCh:
			if got != "broadcast-msg" {
				t.Fatalf("broadcast mismatch: %q", got)
			}
		case <-timeout:
			t.Fatalf("broadcast timed out, %d received", i)
		}
	}
}

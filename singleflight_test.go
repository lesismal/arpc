// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	methodSingleflight       = "/singleflight"
	methodSingleflightKey    = "/singleflightkey"
	methodSingleflightCtx    = "/singleflightctx"
	methodSingleflightAsc    = "/singleflightasync"
	methodSingleflightStruct = "/singleflightstruct"
	singleflightAddr         = "localhost:11003"
)

type sfReq struct {
	ID int
}

// String makes sfReq a fmt.Stringer so the default singleflight key func can
// derive a key from it.
func (r *sfReq) String() string {
	return fmt.Sprintf("sfReq:%d", r.ID)
}

// sfResp is a struct response used to exercise the codec-decode singleflight
// path(where the decode-once optimization matters most).
type sfResp struct {
	ID   int
	Name string
	Tags []string
}

// newSingleflightServer starts a server that counts how many times each method
// is actually invoked and echoes a per-method response after a small delay(so
// concurrent Calls overlap and can be de-duplicated).
func newSingleflightServer(t *testing.T, addr string, hits *int32) *Server {
	svr := NewServer()
	echo := func(ctx *Context) {
		atomic.AddInt32(hits, 1)
		var req sfReq
		ctx.Bind(&req)
		time.Sleep(time.Second / 10)
		ctx.Write(fmt.Sprintf("resp:%d", req.ID))
	}
	for _, m := range []string{methodSingleflight, methodSingleflightKey, methodSingleflightCtx, methodSingleflightAsc} {
		svr.Handler.Handle(m, echo, true)
	}
	svr.Handler.Handle(methodSingleflightStruct, func(ctx *Context) {
		atomic.AddInt32(hits, 1)
		var req sfReq
		ctx.Bind(&req)
		time.Sleep(time.Second / 10)
		ctx.Write(&sfResp{ID: req.ID, Name: fmt.Sprintf("name-%d", req.ID), Tags: []string{"a", "b"}})
	}, true)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	go svr.Serve(ln)
	return svr
}

func TestClient_SingleflightDefaultKey(t *testing.T) {
	var hits int32
	svr := newSingleflightServer(t, singleflightAddr, &hits)
	defer svr.Stop()

	handler := DefaultHandler.Clone()
	handler.Singleflight(methodSingleflight)

	c, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", singleflightAddr, time.Second)
	}, handler)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			req := &sfReq{ID: 1}
			var rsp string
			if err := c.Call(methodSingleflight, req, &rsp, time.Second*3); err != nil {
				t.Errorf("Call error: %v", err)
				return
			}
			if rsp != "resp:1" {
				t.Errorf("unexpected rsp %q, want %q", rsp, "resp:1")
			}
		}()
	}
	wg.Wait()

	// All n concurrent Calls share the same key, so the server should be hit
	// far fewer than n times(ideally once, allow a little slack for timing).
	if got := atomic.LoadInt32(&hits); got >= n {
		t.Fatalf("singleflight did not de-duplicate: server hits=%v, want < %v", got, n)
	}
}

func TestClient_SingleflightCustomKey(t *testing.T) {
	var hits int32
	svr := newSingleflightServer(t, "localhost:11004", &hits)
	defer svr.Stop()

	handler := DefaultHandler.Clone()
	// Key by the request ID; two distinct IDs must not be de-duplicated.
	handler.Singleflight(methodSingleflightKey, func(req interface{}) string {
		return fmt.Sprintf("%d", req.(*sfReq).ID)
	})

	c, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:11004", time.Second)
	}, handler)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	const groups = 2
	const perGroup = 10
	var wg sync.WaitGroup
	wg.Add(groups * perGroup)
	for g := 0; g < groups; g++ {
		id := g + 1
		for i := 0; i < perGroup; i++ {
			go func() {
				defer wg.Done()
				req := &sfReq{ID: id}
				var rsp string
				if err := c.Call(methodSingleflightKey, req, &rsp, time.Second*3); err != nil {
					t.Errorf("Call error: %v", err)
					return
				}
				if want := fmt.Sprintf("resp:%d", id); rsp != want {
					t.Errorf("unexpected rsp %q, want %q", rsp, want)
				}
			}()
		}
	}
	wg.Wait()

	// Two distinct keys => at least 2 real requests, but far fewer than the
	// total number of concurrent callers.
	got := atomic.LoadInt32(&hits)
	if got < groups {
		t.Fatalf("expected at least %v server hits(one per key), got %v", groups, got)
	}
	if got >= groups*perGroup {
		t.Fatalf("singleflight did not de-duplicate: server hits=%v, want < %v", got, groups*perGroup)
	}
}

func TestClient_SingleflightCallContext(t *testing.T) {
	var hits int32
	svr := newSingleflightServer(t, "localhost:11005", &hits)
	defer svr.Stop()

	handler := DefaultHandler.Clone()
	handler.Singleflight(methodSingleflightCtx)

	c, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:11005", time.Second)
	}, handler)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
			defer cancel()
			req := &sfReq{ID: 1}
			var rsp string
			if err := c.CallContext(ctx, methodSingleflightCtx, req, &rsp); err != nil {
				t.Errorf("CallContext error: %v", err)
				return
			}
			if rsp != "resp:1" {
				t.Errorf("unexpected rsp %q, want %q", rsp, "resp:1")
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&hits); got >= n {
		t.Fatalf("singleflight did not de-duplicate CallContext: server hits=%v, want < %v", got, n)
	}
}

func TestClient_SingleflightCallAsync(t *testing.T) {
	var hits int32
	svr := newSingleflightServer(t, "localhost:11006", &hits)
	defer svr.Stop()

	handler := DefaultHandler.Clone()
	handler.Singleflight(methodSingleflightAsc)

	c, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:11006", time.Second)
	}, handler)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	const n = 20
	var (
		wg     sync.WaitGroup
		okCnt  int32
		errCnt int32
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		req := &sfReq{ID: 1}
		err := c.CallAsync(methodSingleflightAsc, req, func(ctx *Context, err error) {
			defer wg.Done()
			if err != nil {
				atomic.AddInt32(&errCnt, 1)
				return
			}
			var rsp string
			if err := ctx.Bind(&rsp); err != nil {
				atomic.AddInt32(&errCnt, 1)
				return
			}
			if rsp != "resp:1" {
				t.Errorf("unexpected rsp %q, want %q", rsp, "resp:1")
				atomic.AddInt32(&errCnt, 1)
				return
			}
			atomic.AddInt32(&okCnt, 1)
		}, time.Second*3)
		if err != nil {
			wg.Done()
			t.Fatalf("CallAsync error: %v", err)
		}
	}
	wg.Wait()

	// Every caller's handler must fire exactly once with the shared response.
	if got := atomic.LoadInt32(&okCnt); got != n {
		t.Fatalf("CallAsync singleflight: %v handlers succeeded, want %v (errs=%v)", got, n, atomic.LoadInt32(&errCnt))
	}
	// But only a few real requests should have reached the server.
	if got := atomic.LoadInt32(&hits); got >= n {
		t.Fatalf("singleflight did not de-duplicate CallAsync: server hits=%v, want < %v", got, n)
	}
}

// TestClient_SingleflightCallAsyncPooled exercises the async fan-out with
// message pooling enabled on the client, so the leader's response Context is
// recycled right after its handler returns. Followers are dispatched via
// AsyncExecute and must still Bind the response correctly from the standalone
// clone(not the recycled Context).
func TestClient_SingleflightCallAsyncPooled(t *testing.T) {
	var hits int32
	svr := newSingleflightServer(t, "localhost:11009", &hits)
	defer svr.Stop()

	handler := DefaultHandler.Clone()
	handler.EnablePool(true)
	handler.Singleflight(methodSingleflightStruct)

	c, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:11009", time.Second)
	}, handler)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	const n = 30
	var (
		wg     sync.WaitGroup
		okCnt  int32
		errCnt int32
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		req := &sfReq{ID: 1}
		err := c.CallAsync(methodSingleflightStruct, req, func(ctx *Context, err error) {
			defer wg.Done()
			if err != nil {
				atomic.AddInt32(&errCnt, 1)
				return
			}
			var rsp sfResp
			if err := ctx.Bind(&rsp); err != nil {
				atomic.AddInt32(&errCnt, 1)
				return
			}
			if rsp.ID != 1 || rsp.Name != "name-1" || len(rsp.Tags) != 2 {
				t.Errorf("unexpected rsp %+v", rsp)
				atomic.AddInt32(&errCnt, 1)
				return
			}
			atomic.AddInt32(&okCnt, 1)
		}, time.Second*3)
		if err != nil {
			wg.Done()
			t.Fatalf("CallAsync error: %v", err)
		}
	}
	wg.Wait()

	if got := atomic.LoadInt32(&okCnt); got != n {
		t.Fatalf("pooled CallAsync singleflight: %v handlers succeeded, want %v (errs=%v)", got, n, atomic.LoadInt32(&errCnt))
	}
	if got := atomic.LoadInt32(&hits); got >= n {
		t.Fatalf("singleflight did not de-duplicate pooled CallAsync: server hits=%v, want < %v", got, n)
	}
}

func TestClient_SingleflightStructResult(t *testing.T) {
	var hits int32
	svr := newSingleflightServer(t, "localhost:11008", &hits)
	defer svr.Stop()

	handler := DefaultHandler.Clone()
	handler.Singleflight(methodSingleflightStruct)

	c, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", "localhost:11008", time.Second)
	}, handler)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			req := &sfReq{ID: 1}
			// Each caller decodes into its own rsp; the leader decodes once and
			// followers copy the shared result.
			var rsp sfResp
			if err := c.Call(methodSingleflightStruct, req, &rsp, time.Second*3); err != nil {
				t.Errorf("Call error: %v", err)
				return
			}
			if rsp.ID != 1 || rsp.Name != "name-1" || len(rsp.Tags) != 2 || rsp.Tags[0] != "a" || rsp.Tags[1] != "b" {
				t.Errorf("unexpected rsp %+v", rsp)
			}
		}()
	}
	wg.Wait()

	// All n concurrent Calls shared one request(and one decode).
	if got := atomic.LoadInt32(&hits); got >= n {
		t.Fatalf("singleflight did not de-duplicate struct result: server hits=%v, want < %v", got, n)
	}
}

func TestHandler_SingleflightKey(t *testing.T) {
	h := NewHandler()

	if _, ok := h.SingleflightKey("/x", &sfReq{ID: 1}); ok {
		t.Fatal("SingleflightKey should report not-enabled before Singleflight is called")
	}

	// Default key uses fmt.Stringer.
	h.Singleflight("/x")
	if key, ok := h.SingleflightKey("/x", &sfReq{ID: 7}); !ok || key != "sfReq:7" {
		t.Fatalf("default key = (%q, %v), want (%q, true)", key, ok, "sfReq:7")
	}

	// Custom key func takes precedence.
	h.Singleflight("/y", func(req interface{}) string {
		return "custom"
	})
	if key, ok := h.SingleflightKey("/y", &sfReq{ID: 7}); !ok || key != "custom" {
		t.Fatalf("custom key = (%q, %v), want (%q, true)", key, ok, "custom")
	}
}

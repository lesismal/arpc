// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type sfReq struct{ ID int }

func (r *sfReq) String() string { return fmt.Sprintf("sfReq:%d", r.ID) }

func TestSingleflightGroup(t *testing.T) {
	var g singleflightGroup
	k := sfKey{method: "m", key: "k"}

	call, leader := g.acquire(k)
	if !leader {
		t.Fatal("the first caller should lead")
	}
	if c2, leader := g.acquire(k); leader || c2 != call {
		t.Fatal("a concurrent caller should follow the same call")
	}
	if _, leader := g.acquire(sfKey{method: "m", key: "k", async: true}); !leader {
		t.Fatal("async callers are grouped apart")
	}

	g.finish(k, call, []byte("data"), nil)
	<-call.done
	if string(call.data) != "data" || call.err != nil {
		t.Fatal("finish should publish the result")
	}
	c3, leader := g.acquire(k)
	if !leader || c3 == call {
		t.Fatal("after finish, a new call starts")
	}
	// Releasing a superseded call does not remove the current one.
	g.release(k, call)
	if c4, leader := g.acquire(k); leader || c4 != c3 {
		t.Fatal("release of a stale call removed the current one")
	}
}

func TestSingleflightAsyncFanout(t *testing.T) {
	call := &singleflightCall{done: make(chan struct{})}
	var got []error
	var mu sync.Mutex
	errResult := errors.New("result")
	for i := 0; i < 3; i++ {
		if !call.addSub(&sfAsyncSub{handler: func(ctx *Context, err error) {
			mu.Lock()
			got = append(got, err)
			mu.Unlock()
		}}) {
			t.Fatal("addSub before fanout should succeed")
		}
	}
	call.fanout(nil, errResult)
	if len(got) != 3 || got[0] != errResult {
		t.Fatalf("fanout delivered %v", got)
	}
	if call.addSub(&sfAsyncSub{}) {
		t.Fatal("addSub after fanout should fail")
	}

	// A sub fires once: by its timer or the fanout, whichever comes first.
	var fired int32
	sub := &sfAsyncSub{handler: func(*Context, error) { atomic.AddInt32(&fired, 1) }}
	sub.timer = time.AfterFunc(time.Hour, func() {})
	sub.fire(nil, ErrTimeout)
	sub.fire(nil, nil)
	if fired != 1 {
		t.Fatalf("fired %v times", fired)
	}
}

// sfServer starts a server whose "/sf" handler counts hits and blocks until
// release is closed.
func sfServer(t *testing.T) (addr string, hits *int32, release chan struct{}) {
	hits = new(int32)
	release = make(chan struct{})
	_, addr = startServer(t, func(h Handler) {
		h.Handle("/sf", func(ctx *Context) {
			atomic.AddInt32(hits, 1)
			<-release
			var req sfReq
			ctx.Bind(&req)
			if req.ID < 0 {
				ctx.Error("negative id")
				return
			}
			ctx.Write(fmt.Sprintf("rsp:%d", req.ID))
		})
	})
	return addr, hits, release
}

func TestSingleflight_Call(t *testing.T) {
	addr, hits, release := sfServer(t)
	c := dialClient(t, addr, func(h Handler) { h.Singleflight("/sf") })

	const n = 10
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var rsp string
			var err error
			if i%2 == 0 {
				err = c.Call("/sf", &sfReq{ID: 1}, &rsp, 2*time.Second)
			} else {
				err = c.CallContext(context.Background(), "/sf", &sfReq{ID: 1}, &rsp)
			}
			if err == nil && rsp != "rsp:1" {
				err = fmt.Errorf("rsp = %q", rsp)
			}
			errs <- err
		}(i)
	}
	waitFor(t, "leader request", func() bool { return atomic.LoadInt32(hits) == 1 })
	time.Sleep(50 * time.Millisecond) // let the followers join
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(hits); got >= n {
		t.Fatalf("%v requests for %v calls, want de-duplication", got, n)
	}

	// Different keys are not merged; errors are shared too.
	var rsp string
	if err := c.Call("/sf", &sfReq{ID: 2}, &rsp, time.Second); err != nil || rsp != "rsp:2" {
		t.Fatalf("Call = %v, %q", err, rsp)
	}
	if err := c.Call("/sf", &sfReq{ID: -1}, &rsp, time.Second); err == nil || err.Error() != "negative id" {
		t.Fatalf("Call = %v", err)
	}
}

func TestSingleflight_FollowerGivesUp(t *testing.T) {
	addr, hits, release := sfServer(t)
	defer close(release)
	c := dialClient(t, addr, func(h Handler) { h.Singleflight("/sf") })

	leader := make(chan error, 1)
	go func() { leader <- c.Call("/sf", &sfReq{ID: 1}, nil, 50*time.Millisecond) }()
	waitFor(t, "leader request", func() bool { return atomic.LoadInt32(hits) == 1 })

	// A follower times out on its own.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := c.CallContext(ctx, "/sf", &sfReq{ID: 1}, nil); err != ErrClientTimeout {
		t.Fatalf("follower = %v, want ErrClientTimeout", err)
	}
	// The leader's error is shared with the followers still waiting.
	follower := make(chan error, 1)
	go func() { follower <- c.Call("/sf", &sfReq{ID: 1}, nil, time.Second) }()
	if err := recvWithin(t, leader, "leader"); err != ErrClientTimeout {
		t.Fatalf("leader = %v, want ErrClientTimeout", err)
	}
	if err := recvWithin(t, follower, "follower"); err != ErrClientTimeout && err != nil {
		t.Fatalf("follower = %v", err)
	}

	// A follower returns when the Client stops.
	go func() { leader <- c.Call("/sf", &sfReq{ID: 3}, nil, time.Second) }()
	waitFor(t, "second leader request", func() bool { return atomic.LoadInt32(hits) >= 2 })
	go func() { follower <- c.Call("/sf", &sfReq{ID: 3}, nil, time.Second) }()
	time.Sleep(20 * time.Millisecond)
	c.Stop()
	if err := recvWithin(t, follower, "follower"); err != ErrClientStopped {
		t.Fatalf("follower = %v, want ErrClientStopped", err)
	}
	recvWithin(t, leader, "leader")
}

func TestSingleflight_CallAsync(t *testing.T) {
	addr, hits, release := sfServer(t)
	c := dialClient(t, addr, func(h Handler) { h.Singleflight("/sf") })

	const n = 10
	type result struct {
		rsp string
		err error
	}
	results := make(chan result, n)
	// CallAsync returns at once, so all calls join the first one while the
	// server holds it.
	for i := 0; i < n; i++ {
		if err := c.CallAsync("/sf", &sfReq{ID: 1}, func(ctx *Context, err error) {
			var rsp string
			if err == nil {
				err = ctx.Bind(&rsp)
			}
			results <- result{rsp, err}
		}, 2*time.Second); err != nil {
			t.Fatalf("CallAsync: %v", err)
		}
	}
	waitFor(t, "leader request", func() bool { return atomic.LoadInt32(hits) == 1 })
	close(release)
	for i := 0; i < n; i++ {
		if r := recvWithin(t, results, "result"); r.err != nil || r.rsp != "rsp:1" {
			t.Fatalf("result %+v", r)
		}
	}
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("%v requests, want 1", got)
	}
}

func TestSingleflight_CallAsyncFollowerTimeout(t *testing.T) {
	addr, _, release := sfServer(t)
	defer close(release)
	c := dialClient(t, addr, func(h Handler) { h.Singleflight("/sf") })

	leader := make(chan error, 1)
	follower := make(chan error, 1)
	if err := c.CallAsync("/sf", &sfReq{ID: 1}, func(_ *Context, err error) { leader <- err }, time.Second); err != nil {
		t.Fatalf("CallAsync: %v", err)
	}
	if err := c.CallAsync("/sf", &sfReq{ID: 1}, func(_ *Context, err error) { follower <- err }, 10*time.Millisecond); err != nil {
		t.Fatalf("CallAsync: %v", err)
	}
	if err := recvWithin(t, follower, "follower timeout"); err != ErrTimeout {
		t.Fatalf("follower = %v, want ErrTimeout", err)
	}
}

func TestSingleflight_CallAsyncLeaderFails(t *testing.T) {
	// When the leader cannot send, it gets the error, its handler is not
	// called, and the call is released for the next caller.
	h := NewHandler()
	h.SetAsyncWrite(false)
	h.Singleflight("/sf")
	c, conn, _ := pipeClient(t, h)
	conn.setFailWrite(true)

	called := make(chan error, 1)
	err := c.CallAsync("/sf", &sfReq{ID: 1}, func(_ *Context, err error) { called <- err }, time.Second)
	if err != errTestWrite {
		t.Fatalf("CallAsync = %v", err)
	}
	assertNoRecv(t, called, 20*time.Millisecond, "leader handler")
	if _, leader := c.sfGroup.acquire(sfKey{method: "/sf", key: "sfReq:1", async: true}); !leader {
		t.Fatal("the failed call should be released")
	}
}

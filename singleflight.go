// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"sync"
	"sync/atomic"
	"time"
)

// sfKey identifies an in-flight singleflight call. Blocking callers
// (Call/CallContext) and async callers (CallAsync) receive results
// differently, so async keeps them in separate groups.
type sfKey struct {
	method string
	key    string
	async  bool
}

// singleflightCall is one in-flight request shared by de-duplicated callers.
//
//   - Blocking callers: the leader sets data/err and closes done; followers
//     wait on done, then read data/err.
//   - Async callers: followers register an sfAsyncSub, and the leader fans its
//     result out to all of them.
type singleflightCall struct {
	// blocking fields
	done chan struct{}
	data []byte
	err  error

	// async fan-out fields
	mu       sync.Mutex
	finished bool
	subs     []*sfAsyncSub
}

// sfAsyncSub is an async follower waiting for the leader's result. Its handler
// is called exactly once, by the leader's fan-out or by its own timeout,
// whichever comes first.
type sfAsyncSub struct {
	handler AsyncHandlerFunc
	timer   *time.Timer
	fired   int32
}

// fire calls the handler if it has not been called yet, and stops the timer.
func (sub *sfAsyncSub) fire(ctx *Context, err error) {
	if atomic.CompareAndSwapInt32(&sub.fired, 0, 1) {
		if sub.timer != nil {
			sub.timer.Stop()
		}
		sub.handler(ctx, err)
	}
}

// singleflightGroup de-duplicates concurrent requests with the same sfKey: the
// first caller (the leader) sends the request, and later callers (followers)
// share its response.
type singleflightGroup struct {
	mu    sync.Mutex
	calls map[sfKey]*singleflightCall
}

// acquire returns the in-flight call for k, creating it if absent. leader
// reports whether it was created by this caller, which must then send the
// request and publish the result via finish, or fanout and release.
func (g *singleflightGroup) acquire(k sfKey) (call *singleflightCall, leader bool) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[sfKey]*singleflightCall)
	}
	if c, ok := g.calls[k]; ok {
		g.mu.Unlock()
		return c, false
	}
	call = &singleflightCall{done: make(chan struct{})}
	g.calls[k] = call
	g.mu.Unlock()
	return call, true
}

// release removes call from the group so that later callers start a new
// request. It does nothing if k now maps to another call.
func (g *singleflightGroup) release(k sfKey, call *singleflightCall) {
	g.mu.Lock()
	if g.calls[k] == call {
		delete(g.calls, k)
	}
	g.mu.Unlock()
}

// finish publishes the result of a blocking leader, releases the call and
// wakes up all followers waiting on done.
func (g *singleflightGroup) finish(k sfKey, call *singleflightCall, data []byte, err error) {
	call.data = data
	call.err = err
	g.release(k, call)
	close(call.done)
}

// addSub registers an async follower. It returns false if the call has
// already finished, in which case the caller should send its own request.
func (call *singleflightCall) addSub(sub *sfAsyncSub) bool {
	call.mu.Lock()
	if call.finished {
		call.mu.Unlock()
		return false
	}
	call.subs = append(call.subs, sub)
	call.mu.Unlock()
	return true
}

// fanout marks the call finished and delivers the async leader's result to
// all registered followers. Followers arriving later fail addSub.
func (call *singleflightCall) fanout(ctx *Context, err error) {
	call.mu.Lock()
	call.finished = true
	subs := call.subs
	call.subs = nil
	call.mu.Unlock()
	for _, sub := range subs {
		sub.fire(ctx, err)
	}
}

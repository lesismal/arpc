// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"sync"
	"sync/atomic"
	"time"
)

// sfKey identifies an in-flight singleflight call. async separates blocking
// callers(Call/CallContext) from asynchronous callers(CallAsync) so the two
// de-duplicate within their own kind, since their result delivery differs.
type sfKey struct {
	method string
	key    string
	async  bool
}

// singleflightCall represents a single in-flight request shared by
// de-duplicated callers.
//
//   - Blocking callers(Call/CallContext): the leader fills data/err and closes
//     done; followers wait on done and then read the shared data/err.
//   - Async callers(CallAsync): followers register an sfAsyncSub and the leader
//     fans its result out to every subscriber.
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

// sfAsyncSub is a CallAsync follower waiting for the leader's result. Its
// handler is invoked exactly once, either by the leader's fan-out or by its own
// timeout, whichever happens first(guarded by fired).
type sfAsyncSub struct {
	handler AsyncHandlerFunc
	timer   *time.Timer
	fired   int32
}

// fire invokes the subscriber's handler at most once and stops its timeout timer.
func (sub *sfAsyncSub) fire(ctx *Context, err error) {
	if atomic.CompareAndSwapInt32(&sub.fired, 0, 1) {
		if sub.timer != nil {
			sub.timer.Stop()
		}
		sub.handler(ctx, err)
	}
}

// singleflightGroup de-duplicates concurrent requests that share the same
// sfKey: the first caller(the leader) issues the real request while later
// callers(followers) wait for and share the leader's response.
type singleflightGroup struct {
	mu    sync.Mutex
	calls map[sfKey]*singleflightCall
}

// acquire returns the in-flight call for k, creating one if absent. leader
// reports whether this caller created it and therefore must drive the real
// request(and eventually publish the result via finish or fanout+release).
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

// release removes call from the map so later callers start a fresh request. It
// is a no-op if the entry was already replaced.
func (g *singleflightGroup) release(k sfKey, call *singleflightCall) {
	g.mu.Lock()
	if g.calls[k] == call {
		delete(g.calls, k)
	}
	g.mu.Unlock()
}

// finish publishes a blocking leader's result, drops the in-flight entry and
// wakes every follower waiting on done.
func (g *singleflightGroup) finish(k sfKey, call *singleflightCall, data []byte, err error) {
	call.data = data
	call.err = err
	g.release(k, call)
	close(call.done)
}

// addSub registers an async follower. It returns false when the leader already
// finished, in which case the caller should fall back to its own request.
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

// fanout delivers the async leader's result to every registered follower
// exactly once and marks the call finished so late followers fall back.
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

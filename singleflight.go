// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"sync"
	"sync/atomic"
	"time"
)

// singleflightCallPool recycles singleflightCall structs, which are allocated
// once per real round-trip(the leader's). Callers share a call through a
// reference count so it is only returned here once every sharing caller is done
// with it(see releaseCall).
var singleflightCallPool = sync.Pool{
	New: func() interface{} {
		return &singleflightCall{}
	},
}

// sfKey identifies an in-flight singleflight call. async separates blocking
// callers(Call/CallContext) from asynchronous callers(CallAsync) so the two
// de-duplicate within their own kind, since their result delivery differs.
type sfKey struct {
	method string
	key    string
	async  bool
}

// singleflightCall is one in-flight request shared by de-duplicated callers.
//
//   - Blocking callers(Call/CallContext): the leader decodes the response once
//     into result(and keeps the raw data as a fallback) and closes done;
//     followers wait on done and then copy the shared result into their own rsp
//     instead of decoding the payload again.
//   - Async callers(CallAsync): followers register an sfAsyncSub and the leader
//     fans its result out to every subscriber.
type singleflightCall struct {
	// blocking fields
	done chan struct{}
	// result is the leader's decoded response holder(a fresh pointer of the
	// leader's rsp type). It is written once before done is closed and only
	// read afterwards, so concurrent follower copies are safe. data is the raw
	// response payload, kept as a fallback for followers whose rsp type differs
	// from the leader's.
	data   []byte
	result interface{}
	err    error

	// async fan-out fields(CallAsync). finished and subs are guarded by the
	// owning singleflightGroup.mu so that publishing the result(capturing subs
	// + removing the call from the group) and registering a follower are single
	// consistent critical sections.
	finished bool
	subs     []*sfAsyncSub

	// refs counts the callers still holding this call: the leader plus every
	// follower that joined while the entry was in the group. It is incremented
	// under singleflightGroup.mu(strictly before the leader removes the entry)
	// and decremented via releaseCall; the last releaser recycles the call to
	// singleflightCallPool, so a pooled call can never be observed by an
	// in-flight follower.
	refs int32
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
		// Join as a follower: take a reference so the shared call is not
		// recycled while we still read it(blocking) or register on it(async).
		// Incrementing under g.mu, i.e. strictly before the leader can remove
		// the entry in finish/finishAsync, guarantees every sharing follower is
		// counted.
		atomic.AddInt32(&c.refs, 1)
		g.mu.Unlock()
		return c, false
	}
	// Become the leader: reuse a pooled call. The blocking path signals
	// completion through done; the async path never touches done, so the
	// channel is only allocated when it is actually needed.
	call = singleflightCallPool.Get().(*singleflightCall)
	call.refs = 1
	if !k.async {
		call.done = make(chan struct{})
	}
	g.calls[k] = call
	g.mu.Unlock()
	return call, true
}

// releaseCall drops one reference held by a caller(leader or follower). The
// last releaser resets the call and returns it to the pool. Because a call is
// only recycled after it has been removed from the group(finish/finishAsync)
// and every sharing caller is done touching it, the pooled object can never be
// observed by an in-flight follower.
func (g *singleflightGroup) releaseCall(call *singleflightCall) {
	if atomic.AddInt32(&call.refs, -1) != 0 {
		return
	}
	*call = singleflightCall{}
	singleflightCallPool.Put(call)
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

// finish publishes a blocking leader's decoded result(and the raw data
// fallback), drops the in-flight entry and wakes every follower waiting on done.
func (g *singleflightGroup) finish(k sfKey, call *singleflightCall, data []byte, result interface{}, err error) {
	call.data = data
	call.result = result
	call.err = err
	g.release(k, call)
	close(call.done)
}

// addSub registers an async follower on call. It returns false when the leader
// already finished, in which case the caller should fall back to its own
// request. It shares the group lock with finishAsync so that a follower either
// joins in time to be fanned out or cleanly falls back.
func (g *singleflightGroup) addSub(call *singleflightCall, sub *sfAsyncSub) bool {
	g.mu.Lock()
	if call.finished {
		g.mu.Unlock()
		return false
	}
	call.subs = append(call.subs, sub)
	g.mu.Unlock()
	return true
}

// finishAsync publishes an async leader's result in a single critical section:
// it marks the call finished, removes it from the group(so late followers start
// a fresh request) and returns the captured followers. The caller fires them
// outside the lock. Marking finished and removing the map entry atomically is
// what guarantees consistency between fan-out and follower registration.
func (g *singleflightGroup) finishAsync(k sfKey, call *singleflightCall) []*sfAsyncSub {
	g.mu.Lock()
	call.finished = true
	subs := call.subs
	call.subs = nil
	if g.calls[k] == call {
		delete(g.calls, k)
	}
	g.mu.Unlock()
	return subs
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// trackingAllocator hands out buffers like a pooling allocator does and
// records frees of buffers it does not consider live, i.e. double frees.
type trackingAllocator struct {
	mu          sync.Mutex
	live        map[*byte]bool
	doubleFrees int
}

func newTrackingAllocator(h Handler) *trackingAllocator {
	a := &trackingAllocator{live: map[*byte]bool{}}
	h.HandleMalloc(a.malloc)
	h.HandleAppend(a.append)
	h.HandleFree(a.free)
	return a
}

func (a *trackingAllocator) malloc(size int) []byte {
	b := make([]byte, size, size+1)
	a.mu.Lock()
	a.live[&b[:1][0]] = true
	a.mu.Unlock()
	return b
}

// append moves to a new buffer and frees the old one when it runs out of
// capacity, like BufferPool.Append does when crossing its big size.
func (a *trackingAllocator) append(b []byte, more ...byte) []byte {
	if cap(b)-len(b) >= len(more) {
		return append(b, more...)
	}
	nb := a.malloc(len(b) + len(more))
	copy(nb, b)
	copy(nb[len(b):], more)
	a.free(b)
	return nb
}

func (a *trackingAllocator) free(b []byte) {
	if cap(b) == 0 {
		return
	}
	p := &b[:1][0]
	a.mu.Lock()
	if !a.live[p] {
		a.doubleFrees++
	}
	delete(a.live, p)
	a.mu.Unlock()
}

func (a *trackingAllocator) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.doubleFrees
}

// The batch send loop must free the buffer it ends up with, not the one it
// started with, which Append may have freed already when it grew.
func TestClient_BatchSendLoopNoDoubleFree(t *testing.T) {
	h := NewHandler()
	h.SetSendBufferSize(1 << 20)
	alloc := newTrackingAllocator(h)
	c, _, p := pipeClient(t, h)

	big := strings.Repeat("x", 1500)
	// The first message blocks the send loop in Write, since the peer does not
	// read yet, so the next ones queue up and are merged into one batch that
	// outgrows the loop's initial 2048-byte buffer.
	for i := 0; i < 4; i++ {
		if err := c.Notify("/n", big, time.Second); err != nil {
			t.Fatalf("Notify: %v", err)
		}
	}
	for i := 0; i < 4; i++ {
		if m := p.read(); m.Method() != "/n" || len(m.Data()) != len(big) {
			t.Fatalf("message %v: method %q, %v bytes", i, m.Method(), len(m.Data()))
		}
	}

	c.Stop()
	// Wait for the send loop to exit and free its buffer.
	time.Sleep(50 * time.Millisecond)
	if n := alloc.count(); n != 0 {
		t.Fatalf("%v double frees", n)
	}
}

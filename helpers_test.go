// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"errors"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
)

// waitTimeout bounds every wait in the tests, so that a bug fails the test
// instead of hanging it.
const waitTimeout = 3 * time.Second

func TestMain(m *testing.M) {
	// The library logs every connect, disconnect and dropped message; keep
	// the test output readable.
	log.SetLevel(log.LevelNone)
	os.Exit(m.Run())
}

// waitFor polls cond until it returns true, failing the test after
// waitTimeout.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// recvWithin receives a value from ch, failing the test after waitTimeout.
func recvWithin[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(waitTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
	var zero T
	return zero
}

// assertNoRecv fails the test if ch yields a value within d.
func assertNoRecv[T any](t *testing.T, ch <-chan T, d time.Duration, what string) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("unexpected %s: %v", what, v)
	case <-time.After(d):
	}
}

// mustPanic fails the test if f does not panic.
func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s: expected panic", what)
		}
	}()
	f()
}

// withDefaultHandler runs f with DefaultHandler replaced by a fresh Handler,
// restoring the original afterwards.
func withDefaultHandler(t *testing.T, f func(h Handler)) {
	t.Helper()
	old := DefaultHandler
	h := NewHandler()
	SetHandler(h)
	defer SetHandler(old)
	f(h)
}

// startServer starts a Server on an ephemeral port with its Handler set up by
// setup, and stops it at the end of the test. The accept loop is running when
// it returns.
func startServer(t *testing.T, setup func(h Handler)) (*Server, string) {
	t.Helper()
	s := NewServer()
	if setup != nil {
		setup(s.Handler)
	}
	return s, serve(t, s)
}

// serve serves s on an ephemeral port until the end of the test, and returns
// the address. The accept loop is running when it returns.
func serve(t *testing.T, s *Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Serve(ln)
	}()
	waitFor(t, "server running", s.isRunning)
	t.Cleanup(func() {
		s.Stop()
		<-done
	})
	return ln.Addr().String()
}

// clientRunning reads c.running under its lock.
func clientRunning(c *Client) bool {
	c.mux.Lock()
	defer c.mux.Unlock()
	return c.running
}

// tcpDialer returns a DialerFunc for addr.
func tcpDialer(addr string) DialerFunc {
	return func() (net.Conn, error) {
		return net.DialTimeout("tcp", addr, time.Second)
	}
}

// dialClient creates a client-role Client connected to addr, with a new
// Handler set up by setup, and stops it at the end of the test. Create it after
// the server so that it is stopped first and does not try to reconnect.
func dialClient(t *testing.T, addr string, setup func(h Handler)) *Client {
	t.Helper()
	h := NewHandler()
	if setup != nil {
		setup(h)
	}
	c, err := NewClient(tcpDialer(addr), h)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(c.Stop)
	return c
}

// testConn wraps a net.Conn and can be told to fail writes.
type testConn struct {
	net.Conn
	failWrite int32
	writes    int32
}

var errTestWrite = errors.New("test: write failed")

func (c *testConn) setFailWrite(fail bool) {
	var v int32
	if fail {
		v = 1
	}
	atomic.StoreInt32(&c.failWrite, v)
}

func (c *testConn) Write(b []byte) (int, error) {
	atomic.AddInt32(&c.writes, 1)
	if atomic.LoadInt32(&c.failWrite) == 1 {
		return 0, errTestWrite
	}
	return c.Conn.Write(b)
}

// peer is the raw far end of a net.Pipe, speaking the arpc wire format.
type peer struct {
	t    *testing.T
	conn net.Conn
}

// read reads one raw message, failing the test after waitTimeout.
func (p *peer) read() *Message {
	p.t.Helper()
	msg, err := p.tryRead(waitTimeout)
	if err != nil {
		p.t.Fatalf("peer read: %v", err)
	}
	return msg
}

// tryRead reads one raw message within d.
func (p *peer) tryRead(d time.Duration) (*Message, error) {
	p.conn.SetReadDeadline(time.Now().Add(d))
	head := make([]byte, HeadLen)
	if _, err := io.ReadFull(p.conn, head); err != nil {
		return nil, err
	}
	bodyLen := Header(head).BodyLen()
	buf := make([]byte, HeadLen+bodyLen)
	copy(buf, head)
	if _, err := io.ReadFull(p.conn, buf[HeadLen:]); err != nil {
		return nil, err
	}
	return &Message{Buffer: buf}, nil
}

// write writes raw bytes, failing the test after waitTimeout.
func (p *peer) write(b []byte) {
	p.t.Helper()
	p.conn.SetWriteDeadline(time.Now().Add(waitTimeout))
	if _, err := p.conn.Write(b); err != nil {
		p.t.Fatalf("peer write: %v", err)
	}
}

// pipeClient creates a server-role Client (no reconnect) on one end of a
// net.Pipe, and returns the Client, its conn and the peer on the other end.
// Writes on a net.Pipe block until the peer reads them.
func pipeClient(t *testing.T, h Handler) (*Client, *testConn, *peer) {
	t.Helper()
	a, b := net.Pipe()
	conn := &testConn{Conn: a}
	c := newClientWithConn(conn, codec.DefaultCodec, h, nil)
	t.Cleanup(func() {
		c.Stop()
		b.Close()
	})
	return c, conn, &peer{t: t, conn: b}
}

// pipeDialer returns a DialerFunc that dials net.Pipes and sends the peer end
// of each on the returned channel.
func pipeDialer(t *testing.T) (DialerFunc, chan *peer) {
	peers := make(chan *peer, 8)
	return func() (net.Conn, error) {
		a, b := net.Pipe()
		t.Cleanup(func() { b.Close() })
		peers <- &peer{t: t, conn: b}
		return a, nil
	}, peers
}

// xorCoder is a MessageCoder flipping every byte after the body length.
type xorCoder struct{}

func (xorCoder) Encode(c *Client, m *Message) *Message {
	for i := HeaderIndexBodyLenEnd; i < len(m.Buffer); i++ {
		m.Buffer[i] ^= 0xFF
	}
	return m
}

func (xorCoder) Decode(c *Client, m *Message) *Message {
	return xorCoder{}.Encode(c, m)
}

// payload is a struct encoded by the default JSON codec.
type payload struct {
	A int
	B string
}

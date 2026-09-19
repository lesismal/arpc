// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// streamMessage builds a stream message as the peer would send it. local is
// set on messages from the side that created the Stream.
func streamMessage(id uint64, method string, v interface{}, local, eof bool) []byte {
	msg := newMessage(CmdStream, method, v, false, false, id, nil, nil, nil)
	msg.SetStreamLocal(local)
	msg.SetStreamEOF(eof)
	return msg.Buffer
}

func streamCount(c *Client) (local, remote int) {
	c.mux.Lock()
	defer c.mux.Unlock()
	return len(c.streamLocalMap), len(c.streamRemoteMap)
}

func TestStream_ServerInitiated(t *testing.T) {
	// Either side can open a Stream; here the server does.
	got := make(chan string, 4)
	svrClients := make(chan *Client, 1)
	_, addr := startServer(t, func(h Handler) {
		h.HandleConnected(func(c *Client) { svrClients <- c })
	})
	dialClient(t, addr, func(h Handler) {
		h.HandleStream("/push", func(s *Stream) {
			for {
				var v payload
				if err := s.Recv(&v); err != nil {
					got <- "eof"
					s.CloseSend()
					return
				}
				got <- v.B
			}
		})
	})
	sc := recvWithin(t, svrClients, "server-side client")

	s := sc.NewStream("/push")
	s2 := sc.NewStream("/push")
	if s.Id() == 0 || s.Id() == s2.Id() {
		t.Fatalf("stream ids %v and %v should be distinct and non-zero", s.Id(), s2.Id())
	}
	if err := s.SendWith(context.Background(), &payload{B: "a"}); err != nil {
		t.Fatalf("SendWith: %v", err)
	}
	if err := s.Send(&payload{B: "b"}, map[interface{}]interface{}{"k": "v"}); err != nil {
		t.Fatalf("Send with values: %v", err)
	}
	if err := s.SendAndCloseWith(context.Background(), &payload{B: "c"}); err != nil {
		t.Fatalf("SendAndCloseWith: %v", err)
	}
	for _, want := range []string{"a", "b", "c", "eof"} {
		if v := recvWithin(t, got, want); v != want {
			t.Fatalf("got %q, want %q", v, want)
		}
	}
	// The send half is closed.
	if err := s.Send("x"); err != ErrStreamClosedSend {
		t.Fatalf("Send after close = %v", err)
	}
	if err := s.SendAndClose("x"); err != ErrStreamClosedSend {
		t.Fatalf("SendAndClose after close = %v", err)
	}
	// The peer's EOF closes the receive half, and the Stream is removed.
	var v string
	if err := s.Recv(&v); err != io.EOF {
		t.Fatalf("Recv = %v, want io.EOF", err)
	}
	waitFor(t, "stream removed", func() bool { l, _ := streamCount(sc); return l == 1 })
}

func TestStream_RecvContext(t *testing.T) {
	c, _, p := pipeClient(t, NewHandler())
	s := c.NewStream("/s")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	var v string
	if err := s.RecvContext(ctx, &v); err != ErrTimeout {
		t.Fatalf("RecvContext = %v, want ErrTimeout", err)
	}

	// Replies to a local Stream come without the local bit.
	p.write(streamMessage(s.Id(), "/s", "one", false, false))
	if err := s.RecvWith(context.Background(), &v); err != nil || v != "one" {
		t.Fatalf("RecvWith = %v, %q", err, v)
	}

	// Empty messages are ignored.
	p.write(streamMessage(s.Id(), "/s", nil, false, false))
	p.write(streamMessage(s.Id(), "/s", "two", false, false))
	if err := s.Recv(&v); err != nil || v != "two" {
		t.Fatalf("Recv = %v, %q", err, v)
	}

	c.Stop()
	if err := s.Recv(&v); err != ErrClientStopped {
		t.Fatalf("Recv after Stop = %v", err)
	}
	if err := s.RecvContext(context.Background(), &v); err != ErrClientStopped {
		t.Fatalf("RecvContext after Stop = %v", err)
	}
	if err := s.Send("x"); err != ErrClientStopped {
		t.Fatalf("Send after Stop = %v", err)
	}
}

func TestStream_CloseRecv(t *testing.T) {
	h := NewHandler()
	var done int32
	h.HandleMessageDone(func(c *Client, m *Message) {
		if m.Cmd() == CmdStream {
			atomic.AddInt32(&done, 1)
		}
	})
	c, _, p := pipeClient(t, h)
	s := c.NewStream("/s")

	// Messages queued before CloseRecv can still be read, then io.EOF.
	p.write(streamMessage(s.Id(), "/s", "queued", false, false))
	waitFor(t, "message queued", func() bool { return len(s.chData) == 1 })
	s.CloseRecvContext(context.Background())
	s.CloseRecv() // only the first close counts
	var v string
	if err := s.Recv(&v); err != nil || v != "queued" {
		t.Fatalf("Recv = %v, %q", err, v)
	}
	if err := s.Recv(&v); err != io.EOF {
		t.Fatalf("Recv = %v, want io.EOF", err)
	}

	// Later messages are released without being queued.
	p.write(streamMessage(s.Id(), "/s", "late", false, false))
	waitFor(t, "late message released", func() bool { return atomic.LoadInt32(&done) == 2 })

	// Closing the send half as well removes the Stream.
	go s.CloseSendContext(context.Background())
	if m := p.read(); m.Cmd() != CmdStream || !m.IsStreamEOF() || !m.IsStreamLocal() || m.Seq() != s.Id() {
		t.Fatalf("unexpected EOF message: cmd=%v eof=%v local=%v", m.Cmd(), m.IsStreamEOF(), m.IsStreamLocal())
	}
	waitFor(t, "stream removed", func() bool { l, _ := streamCount(c); return l == 0 })
	s.CloseSendContext(context.Background()) // only the first close counts
}

func TestStream_RemoteSyncHandler(t *testing.T) {
	// A stream handler registered with async false runs on the read goroutine;
	// a first message with EOF opens and half-closes the Stream at once.
	got := make(chan string, 2)
	h := NewHandler()
	h.HandleStream("/s", func(s *Stream) {
		var v string
		s.Recv(&v)
		got <- v
		if err := s.Recv(&v); err == io.EOF {
			got <- "eof"
		}
	}, false)
	c, _, p := pipeClient(t, h)

	p.write(streamMessage(7, "/s", "only", true, true))
	if v := recvWithin(t, got, "message"); v != "only" {
		t.Fatalf("got %q", v)
	}
	recvWithin(t, got, "eof")
	if _, r := streamCount(c); r != 1 {
		t.Fatalf("%v remote streams, want 1 until the send half closes", r)
	}
}

func TestStream_SendModes(t *testing.T) {
	t.Run("SyncWriteError", func(t *testing.T) {
		h := NewHandler()
		h.SetAsyncWrite(false)
		c, conn, _ := pipeClient(t, h)
		conn.setFailWrite(true)
		if err := c.NewStream("/s").Send("x"); err != errTestWrite {
			t.Fatalf("Send = %v", err)
		}
		waitFor(t, "client stopped", func() bool { return !clientRunning(c) })
	})

	t.Run("Reconnecting", func(t *testing.T) {
		c, _, _ := pipeClient(t, NewHandler())
		s := c.NewStream("/s")
		c.reconnecting = true
		if err := s.Send("x"); err != ErrClientReconnecting {
			t.Fatalf("Send = %v", err)
		}
		c.reconnecting = false
	})

	t.Run("AsyncQueueTimeout", func(t *testing.T) {
		c, _ := fullQueueClient(t, NewHandler())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if err := c.NewStream("/s").SendContext(ctx, "x"); err != ErrTimeout {
			t.Fatalf("SendContext = %v, want ErrTimeout", err)
		}
		if err := c.NewStream("/s").SendAndCloseContext(ctx, "x"); err != ErrTimeout {
			t.Fatalf("SendAndCloseContext = %v, want ErrTimeout", err)
		}
	})

	t.Run("AsyncStopped", func(t *testing.T) {
		c, _ := fullQueueClient(t, NewHandler())
		s := c.NewStream("/s")
		errs := make(chan error, 1)
		go func() { errs <- s.Send("x") }()
		time.Sleep(10 * time.Millisecond)
		c.Stop()
		// The send loop drains the queue when stopping, so the push may get in.
		if err := recvWithin(t, errs, "Send"); err != ErrClientStopped && err != nil {
			t.Fatalf("Send = %v", err)
		}
	})
}

func TestStream_ClearedOnStop(t *testing.T) {
	c, _, p := pipeClient(t, NewHandler())
	s := c.NewStream("/s")
	p.write(streamMessage(9, "/remote", "x", true, false)) // no handler
	c.Stop()
	var v string
	if err := s.Recv(&v); err != ErrClientStopped && err != io.EOF {
		t.Fatalf("Recv = %v", err)
	}
}

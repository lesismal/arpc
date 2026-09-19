// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/lesismal/arpc/codec"
)

func TestHeader(t *testing.T) {
	h := NewHandler()
	head := Header([]byte{5, 0, 0, 0})
	if got := head.BodyLen(); got != 5 {
		t.Fatalf("BodyLen() = %v, want 5", got)
	}

	msg, err := head.message(h)
	if err != nil {
		t.Fatalf("message(): %v", err)
	}
	if msg.Len() != HeadLen+5 || msg.BodyLen() != 5 {
		t.Fatalf("message() len = %v, body len = %v", msg.Len(), msg.BodyLen())
	}

	h.SetMaxBodyLen(4)
	if _, err := head.message(h); err == nil {
		t.Fatal("message() with body len > MaxBodyLen should fail")
	}
}

func TestNewMessage(t *testing.T) {
	values := map[interface{}]interface{}{"k": "v"}
	msg := NewMessage(CmdRequest, "method", "data", true, true, 42, nil, nil, values)

	if msg.handler != DefaultHandler {
		t.Fatal("nil Handler should mean DefaultHandler")
	}
	if msg.Cmd() != CmdRequest || msg.Method() != "method" || string(msg.Data()) != "data" || msg.Seq() != 42 {
		t.Fatalf("unexpected message: cmd=%v method=%q data=%q seq=%v", msg.Cmd(), msg.Method(), msg.Data(), msg.Seq())
	}
	if msg.MethodLen() != len("method") || msg.BodyLen() != len("methoddata") || msg.Len() != HeadLen+len("methoddata") {
		t.Fatalf("unexpected lengths: method=%v body=%v total=%v", msg.MethodLen(), msg.BodyLen(), msg.Len())
	}
	// isError and isAsync are ignored by the exported constructor.
	if msg.IsError() || msg.IsAsync() {
		t.Fatal("NewMessage should ignore isError and isAsync")
	}
	if v, ok := msg.Get("k"); !ok || v != "v" {
		t.Fatalf("Get(k) = %v, %v", v, ok)
	}

	// The internal constructor honors the flags and encodes values with the codec.
	msg = newMessage(CmdResponse, "m", &payload{A: 1, B: "b"}, true, true, 1, nil, codec.DefaultCodec, nil)
	if !msg.IsError() || !msg.IsAsync() {
		t.Fatal("newMessage should set isError and isAsync")
	}
	if string(msg.Data()) != `{"A":1,"B":"b"}` {
		t.Fatalf("Data() = %s", msg.Data())
	}
}

func TestNewWritevMessage(t *testing.T) {
	h := NewHandler()

	t.Run("Empty", func(t *testing.T) {
		msg := newWritevMessage(CmdNotify, "m", nil, false, false, 1, nil, nil, nil)
		if msg.body != nil || msg.bodyPooled || msg.BodyLen() != 1 {
			t.Fatalf("body=%v pooled=%v bodyLen=%v", msg.body, msg.bodyPooled, msg.BodyLen())
		}
		if msg.handler != DefaultHandler {
			t.Fatal("nil Handler should mean DefaultHandler")
		}
	})

	t.Run("Owned", func(t *testing.T) {
		msg := newWritevMessage(CmdNotify, "m", "data", true, true, 1, h, nil, nil)
		if string(msg.body) != "data" || msg.bodyPooled {
			t.Fatalf("body=%q pooled=%v", msg.body, msg.bodyPooled)
		}
		if msg.Len() != HeadLen+1 || msg.BodyLen() != 5 {
			t.Fatalf("header should hold only the method: len=%v bodyLen=%v", msg.Len(), msg.BodyLen())
		}
		if !msg.IsError() || !msg.IsAsync() {
			t.Fatal("flags not set")
		}
	})

	t.Run("BytesCopied", func(t *testing.T) {
		data := []byte("data")
		msg := newWritevMessage(CmdNotify, "m", data, false, false, 1, h, nil, nil)
		data[0] = 'X'
		if string(msg.body) != "data" || !msg.bodyPooled {
			t.Fatalf("body=%q pooled=%v, want a pooled copy", msg.body, msg.bodyPooled)
		}

		// Release frees both buffers.
		var freed int
		h.HandleFree(func(b []byte) { freed++ })
		msg.Release()
		if freed != 2 {
			t.Fatalf("freed %v buffers, want 2", freed)
		}
	})
}

func TestMessage_Flags(t *testing.T) {
	msg := newMessage(CmdStream, "m", "d", false, false, 1, nil, nil, nil)

	msg.SetStreamLocal(true)
	msg.SetStreamEOF(true)
	if !msg.IsStreamLocal() || !msg.IsStreamEOF() {
		t.Fatal("stream bits not set")
	}
	// SetCmd keeps the stream bits, and Cmd masks them out.
	msg.SetCmd(CmdResponse)
	if msg.Cmd() != CmdResponse || !msg.IsStreamLocal() || !msg.IsStreamEOF() {
		t.Fatalf("Cmd() = %v, local=%v eof=%v", msg.Cmd(), msg.IsStreamLocal(), msg.IsStreamEOF())
	}
	msg.SetStreamLocal(false)
	msg.SetStreamEOF(false)
	if msg.IsStreamLocal() || msg.IsStreamEOF() || msg.Cmd() != CmdResponse {
		t.Fatal("stream bits not cleared")
	}

	msg.SetAsync(true)
	if !msg.IsAsync() {
		t.Fatal("async not set")
	}
	msg.SetAsync(false)
	if msg.IsAsync() {
		t.Fatal("async not cleared")
	}

	if msg.Error() != nil {
		t.Fatal("Error() should be nil without the error flag")
	}
	msg.SetError(true)
	if !msg.IsError() || msg.Error() == nil || msg.Error().Error() != "d" {
		t.Fatalf("Error() = %v", msg.Error())
	}
	msg.SetError(false)
	if msg.IsError() {
		t.Fatal("error not cleared")
	}

	msg.SetAsync(true)
	msg.ResetAttrs()
	if msg.Cmd() != CmdNone || msg.IsAsync() || msg.MethodLen() != 0 {
		t.Fatal("ResetAttrs should zero cmd, flags and method length")
	}
}

func TestMessage_FlagBit(t *testing.T) {
	msg := newMessage(CmdNotify, "m", nil, false, false, 1, nil, nil, nil)
	for i := 0; i < 8; i++ {
		if err := msg.SetFlagBit(i, true); err != nil {
			t.Fatalf("SetFlagBit(%v, true): %v", i, err)
		}
		if !msg.IsFlagBitSet(i) {
			t.Fatalf("bit %v not set", i)
		}
		for j := 0; j < 8; j++ {
			if j != i && msg.IsFlagBitSet(j) {
				t.Fatalf("bit %v set along with %v", j, i)
			}
		}
		if err := msg.SetFlagBit(i, false); err != nil {
			t.Fatalf("SetFlagBit(%v, false): %v", i, err)
		}
		if msg.IsFlagBitSet(i) {
			t.Fatalf("bit %v not cleared", i)
		}
	}
	for _, i := range []int{-1, 8, 100} {
		if err := msg.SetFlagBit(i, true); err != ErrInvalidFlagBitIndex {
			t.Fatalf("SetFlagBit(%v) = %v, want ErrInvalidFlagBitIndex", i, err)
		}
		if msg.IsFlagBitSet(i) {
			t.Fatalf("IsFlagBitSet(%v) should be false", i)
		}
	}
	// The flag bits live in the reserved byte and do not touch the header.
	if msg.Cmd() != CmdNotify || msg.Method() != "m" {
		t.Fatal("flag bits corrupted the header")
	}
}

func TestMessage_Fields(t *testing.T) {
	msg := newMessage(CmdRequest, "method", "data", false, false, 1, nil, nil, nil)

	msg.SetSeq(1<<63 + 7)
	if msg.Seq() != 1<<63+7 {
		t.Fatalf("Seq() = %v", msg.Seq())
	}
	msg.SetBodyLen(123)
	if msg.BodyLen() != 123 {
		t.Fatalf("BodyLen() = %v", msg.BodyLen())
	}
	msg.SetMethodLen(3)
	if msg.MethodLen() != 3 || msg.Method() != "met" || msg.method() != "met" || string(msg.Data()) != "hoddata" {
		t.Fatalf("method=%q data=%q", msg.Method(), msg.Data())
	}
}

func TestMessage_Values(t *testing.T) {
	msg := newMessage(CmdRequest, "m", nil, false, false, 1, nil, nil, nil)
	if msg.Values() != nil {
		t.Fatal("Values() should be nil initially")
	}
	if _, ok := msg.Get("k"); ok {
		t.Fatal("Get on empty values should fail")
	}
	msg.Set(nil, "v")
	msg.Set("k", nil)
	if msg.Values() != nil {
		t.Fatal("Set with nil key or value should do nothing")
	}
	msg.Set("k", "v")
	if v, ok := msg.Get("k"); !ok || v != "v" {
		t.Fatalf("Get(k) = %v, %v", v, ok)
	}
	if len(msg.Values()) != 1 {
		t.Fatalf("Values() = %v", msg.Values())
	}
}

func TestMessage_RetainRelease(t *testing.T) {
	h := NewHandler()
	var freed [][]byte
	h.HandleFree(func(b []byte) { freed = append(freed, b) })

	msg := newMessage(CmdNotify, "m", "data", false, false, 1, h, nil, nil)
	if n := msg.Retain(); n != 1 {
		t.Fatalf("Retain() = %v, want 1", n)
	}
	if n := msg.Release(); n != 0 || len(freed) != 0 {
		t.Fatalf("Release() = %v, freed %v; the retained message must not be freed yet", n, len(freed))
	}
	if n := msg.Release(); n != -1 || len(freed) != 1 {
		t.Fatalf("Release() = %v, freed %v; want -1 and 1 free", n, len(freed))
	}
	if msg.Buffer != nil || msg.handler != nil {
		t.Fatal("released message should be reset")
	}

	// A message without a handler is released without freeing.
	(&Message{Buffer: []byte{1}}).Release()

	msg = newMessage(CmdNotify, "m", "data", false, false, 1, h, nil, nil)
	msg.Payback()
	if msg.Buffer != nil || len(freed) != 1 {
		t.Fatal("Payback should reset the message without freeing its buffer")
	}
}

func TestPingPongMessage(t *testing.T) {
	if PingMessage.Cmd() != CmdPing || PongMessage.Cmd() != CmdPong {
		t.Fatal("unexpected ping/pong cmd")
	}
	if PingMessage.Len() != HeadLen || PongMessage.Len() != HeadLen {
		t.Fatal("ping/pong should have no body")
	}
}

func TestCheckMethod(t *testing.T) {
	if err := checkMethod("m"); err != nil {
		t.Fatalf("checkMethod(m): %v", err)
	}
	if err := checkMethod(strings.Repeat("m", MaxMethodLen)); err != nil {
		t.Fatalf("checkMethod(MaxMethodLen): %v", err)
	}
	for _, m := range []string{"", strings.Repeat("m", MaxMethodLen+1)} {
		if err := checkMethod(m); err == nil {
			t.Fatalf("checkMethod(len %v) should fail", len(m))
		}
	}
}

func TestErrors(t *testing.T) {
	// All errors are distinct values with a message.
	errs := []error{
		ErrClientTimeout, ErrClientInvalidTimeoutZero, ErrClientInvalidTimeoutLessThanZero,
		ErrClientInvalidTimeoutZeroWithNonNilCallback, ErrClientOverstock, ErrClientReconnecting,
		ErrClientStopped, ErrClientInvalidPoolDialers, ErrClientInvalidAsyncHandler,
		ErrInvalidRspMessage, ErrMethodNotFound, ErrInvalidFlagBitIndex,
		ErrContextResponseToNotify, ErrStreamClosedSend, ErrTimeout,
	}
	for i, a := range errs {
		if a.Error() == "" {
			t.Fatalf("error %v has an empty message", i)
		}
		for j, b := range errs {
			if i != j && errors.Is(a, b) {
				t.Fatalf("errors %v and %v are the same", i, j)
			}
		}
	}
}

func TestMessage_WireRoundTrip(t *testing.T) {
	// A message written by newWritevMessage has the same wire bytes as the
	// contiguous one.
	a := newMessage(CmdRequest, "method", []byte("data"), false, true, 9, nil, nil, nil)
	b := newWritevMessage(CmdRequest, "method", []byte("data"), false, true, 9, nil, nil, nil)
	wire := append(append([]byte{}, b.Buffer...), b.body...)
	if !bytes.Equal(a.Buffer, wire) {
		t.Fatalf("wire mismatch:\n%v\n%v", a.Buffer, wire)
	}
}

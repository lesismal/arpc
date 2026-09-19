// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"errors"
	"testing"
	"time"

	"github.com/lesismal/arpc/codec"
)

func newTestContext(c *Client, cmd byte, data interface{}, handlers ...HandlerFunc) *Context {
	if c == nil {
		c = &Client{Codec: codec.DefaultCodec, Handler: NewHandler()}
	}
	msg := newMessage(cmd, "m", data, false, false, 7, c.Handler, c.Codec, nil)
	return newContext(c, msg, handlers)
}

func TestContext_Values(t *testing.T) {
	ctx := newTestContext(nil, CmdRequest, nil)
	if _, ok := ctx.Get("k"); ok {
		t.Fatal("Get on empty values should fail")
	}
	if ctx.Value("k") != nil {
		t.Fatal("Value on empty values should be nil")
	}
	ctx.Set(nil, "v")
	ctx.Set("k", nil)
	if ctx.Values() != nil {
		t.Fatal("Set with nil key or value should do nothing")
	}
	ctx.Set("k", "v")
	if v, ok := ctx.Get("k"); !ok || v != "v" {
		t.Fatalf("Get(k) = %v, %v", v, ok)
	}
	if ctx.Value("k") != "v" || ctx.Values()["k"] != "v" {
		t.Fatal("Value/Values mismatch")
	}
	if (&Context{}).Values() != nil {
		t.Fatal("Values without a Message should be nil")
	}
}

func TestContext_StdContext(t *testing.T) {
	ctx := newTestContext(nil, CmdRequest, nil)
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("Deadline should not be set")
	}
	if ctx.Done() != nil || ctx.Err() != nil {
		t.Fatal("Context should never be done")
	}
}

func TestContext_BodyBind(t *testing.T) {
	ctx := newTestContext(nil, CmdRequest, &payload{A: 1, B: "b"})
	if string(ctx.Body()) != `{"A":1,"B":"b"}` {
		t.Fatalf("Body() = %s", ctx.Body())
	}

	var p payload
	if err := ctx.Bind(&p); err != nil || p != (payload{A: 1, B: "b"}) {
		t.Fatalf("Bind(struct) = %v, %+v", err, p)
	}
	var s string
	if err := ctx.Bind(&s); err != nil || s != `{"A":1,"B":"b"}` {
		t.Fatalf("Bind(string) = %v, %q", err, s)
	}
	var b []byte
	if err := ctx.Bind(&b); err != nil || string(b) != s {
		t.Fatalf("Bind(bytes) = %v, %q", err, b)
	}
	if err := ctx.Bind(nil); err != nil {
		t.Fatalf("Bind(nil) = %v", err)
	}
	var n int
	if err := ctx.Bind(&n); err == nil {
		t.Fatal("Bind with a codec error should fail")
	}

	ctx.Message.SetError(true)
	if err := ctx.Bind(&s); err == nil || err.Error() != `{"A":1,"B":"b"}` {
		t.Fatalf("Bind on an error message = %v", err)
	}
}

func TestContext_NextAbort(t *testing.T) {
	var trace []int
	ctx := newTestContext(nil, CmdRequest, nil,
		func(ctx *Context) {
			trace = append(trace, 1)
			ctx.Next()
			trace = append(trace, 4)
		},
		func(ctx *Context) { trace = append(trace, 2) },
		func(ctx *Context) { trace = append(trace, 3); ctx.Abort() },
		func(ctx *Context) { trace = append(trace, 99) },
	)
	// Each Next runs one handler, which may run the rest of the chain by
	// calling Next itself; Abort skips the rest.
	ctx.Next()
	ctx.Next()
	ctx.Next()
	want := []int{1, 2, 4, 3}
	if len(trace) != len(want) {
		t.Fatalf("trace = %v, want %v", trace, want)
	}
	for i := range want {
		if trace[i] != want[i] {
			t.Fatalf("trace = %v, want %v", trace, want)
		}
	}
}

func TestContext_WriteToNotify(t *testing.T) {
	for _, async := range []bool{true, false} {
		h := NewHandler()
		h.SetAsyncWrite(async)
		c, _, _ := pipeClient(t, h)
		ctx := newTestContext(c, CmdNotify, nil)
		if err := ctx.Write("x"); err != ErrContextResponseToNotify {
			t.Fatalf("async=%v: Write = %v", async, err)
		}
		if err := ctx.Error("x"); err != ErrContextResponseToNotify {
			t.Fatalf("async=%v: Error = %v", async, err)
		}
	}
}

func TestContext_Write(t *testing.T) {
	for _, async := range []bool{true, false} {
		h := NewHandler()
		h.SetAsyncWrite(async)
		h.UseCoder(xorCoder{})
		c, _, p := pipeClient(t, h)

		check := func(name string, write func(ctx *Context) error, wantErr bool, wantData string) {
			t.Helper()
			ctx := newTestContext(c, CmdRequest, nil)
			ctx.Message.SetAsync(true)
			done := make(chan error, 1)
			go func() { done <- write(ctx) }()
			rsp := xorCoder{}.Decode(nil, p.read())
			if err := recvWithin(t, done, name); err != nil {
				t.Fatalf("async=%v %s: %v", async, name, err)
			}
			if rsp.Cmd() != CmdResponse || rsp.Seq() != 7 || !rsp.IsAsync() || rsp.Method() != "m" {
				t.Fatalf("async=%v %s: cmd=%v seq=%v async=%v", async, name, rsp.Cmd(), rsp.Seq(), rsp.IsAsync())
			}
			if rsp.IsError() != wantErr || string(rsp.Data()) != wantData {
				t.Fatalf("async=%v %s: isError=%v data=%q", async, name, rsp.IsError(), rsp.Data())
			}
			// ResponseError is only recorded with AsyncWrite.
			if async && wantErr && ctx.ResponseError() == nil {
				t.Fatalf("async=%v %s: ResponseError not recorded", async, name)
			}
			if !wantErr && ctx.ResponseError() != nil {
				t.Fatalf("async=%v %s: unexpected ResponseError %v", async, name, ctx.ResponseError())
			}
		}

		check("Write", func(ctx *Context) error { return ctx.Write("ok") }, false, "ok")
		check("WriteWithTimeout", func(ctx *Context) error { return ctx.WriteWithTimeout("ok", time.Second) }, false, "ok")
		check("WriteError", func(ctx *Context) error { return ctx.Write(errors.New("bad")) }, true, "bad")
		check("Error", func(ctx *Context) error { return ctx.Error("bad") }, true, "bad")
		check("ErrorNil", func(ctx *Context) error { return ctx.Error(nil) }, false, "")
	}
}

func TestContext_WriteDirectlyFailures(t *testing.T) {
	h := NewHandler()
	h.SetAsyncWrite(false)
	var dropped int
	h.HandleMessageDropped(func(c *Client, m *Message) { dropped++ })
	c, conn, _ := pipeClient(t, h)

	// Dropped while reconnecting.
	c.reconnecting = true
	if err := newTestContext(c, CmdRequest, nil).Write("x"); err != ErrClientReconnecting || dropped != 1 {
		t.Fatalf("Write while reconnecting = %v, dropped %v", err, dropped)
	}
	c.reconnecting = false

	// A write error closes the conn.
	conn.setFailWrite(true)
	if err := newTestContext(c, CmdRequest, nil).Write("x"); err != errTestWrite {
		t.Fatalf("Write with a failing conn = %v", err)
	}
	waitFor(t, "client stopped", func() bool { return !clientRunning(c) })
}

func TestContext_Release(t *testing.T) {
	h := NewHandler()
	var freed int
	h.HandleFree(func([]byte) { freed++ })
	c := &Client{Codec: codec.DefaultCodec, Handler: h}
	ctx := newTestContext(c, CmdRequest, "x")
	ctx.Release()
	if freed != 1 || ctx.Message != nil || ctx.Client != nil {
		t.Fatalf("Release: freed %v, ctx %+v", freed, ctx)
	}
}

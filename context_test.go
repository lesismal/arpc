// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"testing"

	"github.com/lesismal/arpc/codec"
)

func TestContext_Get(t *testing.T) {
	ctx := &Context{Message: &Message{}}
	if v, ok := ctx.Get("key"); ok {
		t.Fatalf("Context.Get() error, returns %v, want nil", v)
	}
	values := ctx.Values()
	if len(values) > 0 {
		t.Fatalf("invalid Context.Values() length, returns %v, want 0", len(values))
	}
}

func TestContext_Set(t *testing.T) {
	key := "key"
	value := "value"

	ctx := &Context{Message: &Message{}}
	ctx.Set(key, nil)
	cv, ok := ctx.Get(key)
	if ok {
		t.Fatalf("Context.Get() failed: Get '%v', want nil", cv)
	}

	ctx.Set(key, value)
	cv, ok = ctx.Get(key)
	if !ok {
		t.Fatalf("Context.Get() failed: Get nil, want '%v'", value)
	}
	if cv != value {
		t.Fatalf("Context.Get() failed: Get '%v', want '%v'", cv, value)
	}
}

func TestContext_Body(t *testing.T) {
	bodyValue := "body"
	ctx := &Context{
		Client:  &Client{Codec: codec.DefaultCodec},
		Message: newMessage(CmdRequest, "method", bodyValue, false, false, 0, DefaultHandler, codec.DefaultCodec, nil),
	}
	if string(ctx.Body()) != bodyValue {
		t.Fatalf("Context.Body() = %v, want %v", string(ctx.Body()), bodyValue)
	}
}

func TestContext_Bind(t *testing.T) {
	ctx := &Context{
		Client:  &Client{Codec: codec.DefaultCodec},
		Message: newMessage(CmdRequest, "method", "data", true, false, 0, DefaultHandler, codec.DefaultCodec, nil),
	}
	if err := ctx.Bind(nil); err == nil {
		t.Fatalf("Context.Bind() error = nil, want %v", err)
	}
}

func TestContext_Abort(t *testing.T) {
	ok := false
	h1 := func(ctx *Context) {
		ctx.Abort()
	}
	h2 := func(ctx *Context) {
		ok = true
	}
	ctx := &Context{handlers: []HandlerFunc{h1, h2}}
	ctx.Next()
	if ok {
		t.Fatalf("Context.Abort() ok != false, have %v", ok)
	}
}

func TestContext_Deadline(t *testing.T) {
	ctx := &Context{}
	if deadline, ok := ctx.Deadline(); !deadline.IsZero() || ok {
		t.Fatalf("Context.Deadline() err, have %v, %v", ok, deadline.IsZero())
	}
}

func TestContext_Done(t *testing.T) {
	ctx := &Context{}
	if done := ctx.Done(); done != nil {
		t.Fatalf("Context.Bind() done != nil, have %v", done)
	}
}

func TestContext_Err(t *testing.T) {
	ctx := &Context{}
	ctx.Err()
	if err := ctx.Err(); err != nil {
		t.Fatalf("Context.Err() error != nil, have %v", err)
	}
}

func TestContext_Value(t *testing.T) {
	ctx := &Context{Message: &Message{}}
	if value := ctx.Value(3); value != nil {
		t.Fatalf("Context.Value() value != nil, have %v", value)
	}
	ctx.Set("key", "value")
	if value := ctx.Value("key"); value != "value" {
		t.Fatalf("Context.Value() value != 'value', have %v", value)
	}
}

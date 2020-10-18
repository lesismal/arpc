// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"testing"

	"github.com/lesismal/arpc/codec"
)

func TestContext_Get(t *testing.T) {
	ctx := &Context{}
	if v, ok := ctx.Get("key"); ok {
		t.Fatalf("Context.Get() error, returns %v, want nil", v)
	}
}

func TestContext_Set(t *testing.T) {
	key := "key"
	value := "value"

	ctx := &Context{}
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

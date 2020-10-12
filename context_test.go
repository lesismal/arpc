// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"reflect"
	"testing"

	"github.com/lesismal/arpc/codec"
)

func TestContext_Body(t *testing.T) {
	ctx := &Context{
		Client:  &Client{Codec: codec.DefaultCodec},
		Message: newMessage(CmdRequest, "method", "data", DefaultHandler, codec.DefaultCodec),
	}
	if got := ctx.Body(); !reflect.DeepEqual(got, []byte("data")) {
		t.Errorf("Context.Body() = %v, want %v", got, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	}
}

func TestContext_Bind(t *testing.T) {
	ctx := &Context{
		Client:  &Client{Codec: codec.DefaultCodec},
		Message: newMessage(CmdRequest, "method", "data", DefaultHandler, codec.DefaultCodec),
	}
	ctx.Message.SetError(true)
	if err := ctx.Bind(nil); err == nil {
		t.Errorf("Context.Bind() error = nil, want %v", err)
	}
}

func TestContext_Write(t *testing.T) {
	ctx := &Context{
		Client:  &Client{Codec: codec.DefaultCodec},
		Message: newMessage(CmdNotify, "method", "data", DefaultHandler, codec.DefaultCodec),
	}
	if err := ctx.Write(nil); err != ErrShouldOnlyResponseToRequestMessage {
		t.Errorf("Context.Write() error = %v, wantErr %v", err, ErrShouldOnlyResponseToRequestMessage)
	}
}

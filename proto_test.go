// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"reflect"
	"testing"

	"github.com/lesismal/arpc/codec"
)

func TestHeader_BodyLen(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if msg.BodyLen() != 10 {
		t.Fatalf("Header.BodyLen() = %v, want %v", msg.BodyLen(), 10)
	}
}

func TestHeader_message(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if msg.BodyLen() != 10 {
		t.Fatalf("Header.BodyLen() = %v, want %v", msg.BodyLen(), 10)
	}
	head := Header(msg.Buffer[:HeadLen])
	msg2, err := head.message(DefaultHandler)
	if err != nil {
		t.Fatalf("Header.message() error = %v", err)
	}
	if len(msg.Buffer) != len(msg2.Buffer) {
		t.Fatalf("len(Header.message()) = %v, want %v", len(msg2.Buffer), len(msg.Buffer))
	}

	head[0], head[1], head[2], head[3] = 0xFF, 0xFF, 0xFF, 0xFF
	_, err = head.message(DefaultHandler)
	if err == nil {
		t.Fatalf("Header.message() error = nil")
	}
}

func TestMessage_Cmd(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.Cmd(); got != CmdRequest {
		t.Fatalf("Message.Cmd() = %v, want %v", got, CmdRequest)
	}

	msg = newMessage(CmdResponse, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.Cmd(); got != CmdResponse {
		t.Fatalf("Message.Cmd() = %v, want %v", got, CmdResponse)
	}

	msg = newMessage(CmdNotify, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.Cmd(); got != CmdNotify {
		t.Fatalf("Message.Cmd() = %v, want %v", got, CmdNotify)
	}
}

func TestMessage_Async(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.IsAsync(); got != false {
		t.Fatalf("Message.Async() = %v, want %v", got, 0)
	}
	msg.SetAsync(true)
	if got := msg.IsAsync(); got != true {
		t.Fatalf("Message.Async() = %v, want %v", got, 1)
	}
}

func TestMessage_IsAsync(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.IsAsync(); got != false {
		t.Fatalf("Message.IsAsync() = %v, want %v", got, false)
	}
	msg.SetAsync(true)
	if got := msg.IsAsync(); got != true {
		t.Fatalf("Message.IsAsync() = %v, want %v", got, true)
	}
}

func TestMessage_IsError(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.IsError(); got != false {
		t.Fatalf("Message.IsError() = %v, want %v", got, false)
	}
	msg.SetError(true)
	if got := msg.IsError(); got != true {
		t.Fatalf("Message.IsError() = %v, want %v", got, true)
	}
}

func TestMessage_Error(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.Error(); got != nil {
		t.Fatalf("Message.Error() = %v, want %v", got, nil)
	}
}

func TestMessage_Values(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	values := msg.Values()
	if len(values) > 0 {
		t.Fatalf("invalid Message.Values() length, returns %v, want 0", len(values))
	}
}

func TestMessage_SetFlagBit(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	for i := 0; i < 8; i++ {
		if err := msg.SetFlagBit(i, true); err != nil {
			t.Fatalf("Message.SetFlagBit() error: %v, want nil", err)
		}
	}
	for i := 10; i < 16; i++ {
		if err := msg.SetFlagBit(i, true); err == nil {
			t.Fatalf("Message.SetFlagBit() failed, return nil error, want %v", ErrInvalidFlagBitIndex)
		}
	}
}

func TestMessage_IsFlagBitSet(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	for i := 0; i < 8; i++ {
		if err := msg.SetFlagBit(i, true); err != nil {
			t.Fatalf("Message.SetFlagBit() error: %v, want nil", err)
		}
		if !msg.IsFlagBitSet(i) {
			t.Fatalf("Message.GetFlagBit() returns false, want true")
		}

		if err := msg.SetFlagBit(i, false); err != nil {
			t.Fatalf("Message.SetFlagBit() error: %v, want nil", err)
		}
		if msg.IsFlagBitSet(i) {
			t.Fatalf("Message.GetFlagBit() returns true, want false")
		}
	}
	for i := 8; i < 16; i++ {
		if err := msg.SetFlagBit(i, true); err == nil {
			t.Fatalf("Message.SetFlagBit() returns nil error, want %v", ErrInvalidFlagBitIndex)
		}
		if msg.IsFlagBitSet(i) {
			t.Fatalf("Message.GetFlagBit() returns true, want false")
		}

		if err := msg.SetFlagBit(i, false); err == nil {
			t.Fatalf("Message.SetFlagBit() returns nil error, want %v", ErrInvalidFlagBitIndex)
		}
		if msg.IsFlagBitSet(i) {
			t.Fatalf("Message.GetFlagBit() returns true, want false")
		}
	}
}

func TestMessage_MethodLen(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.MethodLen(); got != 5 {
		t.Fatalf("Message.MethodLen() = %v, want %v", got, 5)
	}
}

func TestMessage_Method(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.Method(); got != "hello" {
		t.Fatalf("Message.Method() = %v, want %v", got, "hello")
	}
}

func TestMessage_BodyLen(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.BodyLen(); got != 10 {
		t.Fatalf("Message.BodyLen() = %v, want %v", got, 10)
	}
	msg.SetBodyLen(100)
	if got := msg.BodyLen(); got != 100 {
		t.Fatalf("Message.BodyLen() = %v, want %v", got, 100)
	}
}

func TestMessage_Seq(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.Seq(); got != 0 {
		t.Fatalf("Message.Seq() = %v, want %v", got, 0)
	}
}

func TestMessage_Data(t *testing.T) {
	msg := newMessage(CmdRequest, "hello", "hello", false, false, 0, DefaultHandler, codec.DefaultCodec, nil)
	if got := msg.Data(); !reflect.DeepEqual(got, []byte("hello")) {
		t.Fatalf("Message.Data() = %v, want %v", got, []byte("hello"))
	}
}

func TestMessage_Get(t *testing.T) {
	msg := &Message{}
	if v, ok := msg.Get("key"); ok {
		t.Fatalf("Message.Get() error, returns %v, want nil", v)
	}
}

func TestMessage_Set(t *testing.T) {
	key := "key"
	value := "value"

	msg := &Message{}
	msg.Set(key, nil)
	cv, ok := msg.Get(key)
	if ok {
		t.Fatalf("Message.Get() failed: Get '%v', want nil", cv)
	}

	msg.Set(key, value)
	cv, ok = msg.Get(key)
	if !ok {
		t.Fatalf("Message.Get() failed: Get nil, want '%v'", value)
	}
	if cv != value {
		t.Fatalf("Message.Get() failed: Get '%v', want '%v'", cv, value)
	}
}

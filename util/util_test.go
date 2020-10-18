// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"errors"
	"reflect"
	"testing"

	"github.com/lesismal/arpc/codec"
)

func Test_StrToBytes(t *testing.T) {
	if got := StrToBytes("hello world"); !reflect.DeepEqual(got, []byte("hello world")) {
		t.Errorf("StrToBytes() = %v, want %v", got, []byte("hello world"))
	}
}

func Test_BytesToStr(t *testing.T) {
	if got := BytesToStr([]byte("hello world")); got != "hello world" {
		t.Errorf("BytesToStr() = %v, want %v", got, "hello world")
	}
}

func Test_ValueToBytes(t *testing.T) {
	if got := ValueToBytes(codec.DefaultCodec, nil); got != nil {
		t.Errorf("ValueToBytes() = %v, want %v", got, nil)
	}
	if got := ValueToBytes(codec.DefaultCodec, "test"); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	str := "test"
	if got := ValueToBytes(codec.DefaultCodec, &str); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := ValueToBytes(codec.DefaultCodec, []byte("test")); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	bts := []byte("test")
	if got := ValueToBytes(codec.DefaultCodec, &bts); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := ValueToBytes(codec.DefaultCodec, errors.New("test")); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	err := errors.New("test")
	if got := ValueToBytes(nil, &err); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := ValueToBytes(&codec.JSONCodec{}, &struct{ I int }{I: 3}); !reflect.DeepEqual(got, []byte(`{"I":3}`)) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte(`{"I":3}`))
	}
	if got := ValueToBytes(nil, 0); len(got) < 0 {
		t.Errorf("ValueToBytes() len = %v, want 0", len(got))
	}
}

func Test_Safe(t *testing.T) {
	Safe(func() {})
}

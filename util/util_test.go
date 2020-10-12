// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"errors"
	"reflect"
	"testing"
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
	if got := ValueToBytes(Defaultcodec.Codec, nil); got != nil {
		t.Errorf("ValueToBytes() = %v, want %v", got, nil)
	}
	if got := ValueToBytes(Defaultcodec.Codec, "test"); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	str := "test"
	if got := ValueToBytes(Defaultcodec.Codec, &str); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := ValueToBytes(Defaultcodec.Codec, []byte("test")); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	bts := []byte("test")
	if got := ValueToBytes(Defaultcodec.Codec, &bts); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := ValueToBytes(Defaultcodec.Codec, errors.New("test")); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	err := errors.New("test")
	if got := ValueToBytes(nil, &err); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := ValueToBytes(&JSONcodec.Codec{}, &struct{ I int }{I: 3}); !reflect.DeepEqual(got, []byte(`{"I":3}`)) {
		t.Errorf("ValueToBytes() = %v, want %v", got, []byte(`{"I":3}`))
	}
	if got := ValueToBytes(nil, 0); len(got) < 0 {
		t.Errorf("ValueToBytes() len = %v, want 0", len(got))
	}
}

func Test_GetBuffer(t *testing.T) {
	if got := GetBuffer(100); len(got) != 100 {
		t.Errorf("len(GetBuffer(100)) = %v, want %v", len(got), 100)
	}
}

func Test_Safe(t *testing.T) {
	Safe(func() {})
}

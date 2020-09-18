// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"errors"
	"reflect"
	"testing"
)

func Test_strToBytes(t *testing.T) {
	if got := strToBytes("hello world"); !reflect.DeepEqual(got, []byte("hello world")) {
		t.Errorf("strToBytes() = %v, want %v", got, []byte("hello world"))
	}
}

func Test_bytesToStr(t *testing.T) {
	if got := bytesToStr([]byte("hello world")); got != "hello world" {
		t.Errorf("bytesToStr() = %v, want %v", got, "hello world")
	}
}

func Test_valueToBytes(t *testing.T) {
	if got := valueToBytes(DefaultCodec, nil); got != nil {
		t.Errorf("valueToBytes() = %v, want %v", got, nil)
	}
	if got := valueToBytes(DefaultCodec, "test"); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("valueToBytes() = %v, want %v", got, []byte("test"))
	}
	str := "test"
	if got := valueToBytes(DefaultCodec, &str); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("valueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := valueToBytes(DefaultCodec, []byte("test")); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("valueToBytes() = %v, want %v", got, []byte("test"))
	}
	bts := []byte("test")
	if got := valueToBytes(DefaultCodec, &bts); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("valueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := valueToBytes(DefaultCodec, errors.New("test")); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("valueToBytes() = %v, want %v", got, []byte("test"))
	}
	err := errors.New("test")
	if got := valueToBytes(nil, &err); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("valueToBytes() = %v, want %v", got, []byte("test"))
	}
	if got := valueToBytes(&JSONCodec{}, &struct{ I int }{I: 3}); !reflect.DeepEqual(got, []byte(`{"I":3}`)) {
		t.Errorf("valueToBytes() = %v, want %v", got, []byte(`{"I":3}`))
	}
	if got := valueToBytes(nil, 0); len(got) < 0 {
		t.Errorf("valueToBytes() len = %v, want 0", len(got))
	}
}

func Test_memGet(t *testing.T) {
	if got := memGet(100); len(got) != 100 {
		t.Errorf("len(memGet(100)) = %v, want %v", len(got), 100)
	}
}

func Test_safe(t *testing.T) {
	safe(func() {})
}

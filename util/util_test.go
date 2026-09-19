// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"bytes"
	"errors"
	"testing"

	acodec "github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
)

func init() {
	log.SetLevel(log.LevelNone)
}

type payload struct {
	A int
	B string
}

func TestStrBytes(t *testing.T) {
	if b := StrToBytes("hello"); string(b) != "hello" || len(b) != 5 || cap(b) != 5 {
		t.Fatalf("StrToBytes = %q", b)
	}
	if s := BytesToStr([]byte("hello")); s != "hello" {
		t.Fatalf("BytesToStr = %q", s)
	}
	if b := StrToBytes(""); len(b) != 0 {
		t.Fatalf("StrToBytes(\"\") = %q", b)
	}
}

func TestSafe(t *testing.T) {
	ran := false
	Safe(func() { ran = true })
	if !ran {
		t.Fatal("Safe should call f")
	}
	// A panic is recovered.
	Safe(func() { panic("boom") })
}

func TestValueToBytes(t *testing.T) {
	bs := []byte("bytes")
	s := "string"
	err := errors.New("error")
	codec := &acodec.JSONCodec{}

	for _, tc := range []struct {
		name  string
		v     interface{}
		want  string
		owned bool
	}{
		{"nil", nil, "", true},
		{"[]byte", bs, "bytes", false},
		{"*[]byte", &bs, "bytes", false},
		{"string", s, "string", true},
		{"*string", &s, "string", true},
		{"error", err, "error", true},
		{"*error", &err, "error", true},
		{"struct", &payload{A: 1, B: "b"}, `{"A":1,"B":"b"}`, true},
	} {
		if got := ValueToBytes(codec, tc.v); string(got) != tc.want {
			t.Fatalf("ValueToBytes(%s) = %q, want %q", tc.name, got, tc.want)
		}
		got, owned := ValueToBytesOwned(codec, tc.v)
		if string(got) != tc.want || owned != tc.owned {
			t.Fatalf("ValueToBytesOwned(%s) = %q, %v, want %q, %v", tc.name, got, owned, tc.want, tc.owned)
		}
	}

	// []byte is returned without copying.
	if got := ValueToBytes(nil, bs); &got[0] != &bs[0] {
		t.Fatal("ValueToBytes should not copy a []byte")
	}

	// A nil codec means the default one; an encoding error gives nil.
	if got := ValueToBytes(nil, 1); string(got) != "1" {
		t.Fatalf("ValueToBytes with nil codec = %q", got)
	}
	if got, _ := ValueToBytesOwned(nil, 1); string(got) != "1" {
		t.Fatalf("ValueToBytesOwned with nil codec = %q", got)
	}
	if got := ValueToBytes(codec, make(chan int)); got != nil {
		t.Fatalf("ValueToBytes of an unencodable value = %q", got)
	}
	if got, owned := ValueToBytesOwned(codec, make(chan int)); got != nil || !owned {
		t.Fatalf("ValueToBytesOwned of an unencodable value = %q, %v", got, owned)
	}
}

func TestBytesToValue(t *testing.T) {
	data := []byte(`{"A":1,"B":"b"}`)
	codec := &acodec.JSONCodec{}

	var b []byte
	if err := BytesToValue(codec, data, &b); err != nil || !bytes.Equal(b, data) {
		t.Fatalf("BytesToValue(*[]byte) = %v, %q", err, b)
	}
	b[0] = 'X'
	if data[0] != '{' {
		t.Fatal("BytesToValue should copy into *[]byte")
	}

	var s string
	if err := BytesToValue(codec, data, &s); err != nil || s != string(data) {
		t.Fatalf("BytesToValue(*string) = %v, %q", err, s)
	}
	var e error
	if err := BytesToValue(codec, []byte("oops"), &e); err != nil || e == nil || e.Error() != "oops" {
		t.Fatalf("BytesToValue(*error) = %v, %v", err, e)
	}
	var p payload
	if err := BytesToValue(nil, data, &p); err != nil || p != (payload{A: 1, B: "b"}) {
		t.Fatalf("BytesToValue(struct) = %v, %+v", err, p)
	}
	if err := BytesToValue(codec, data, nil); err != nil {
		t.Fatalf("BytesToValue(nil) = %v", err)
	}
	var n int
	if err := BytesToValue(codec, data, &n); err == nil {
		t.Fatal("BytesToValue with a decoding error should fail")
	}
}

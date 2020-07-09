// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"fmt"
	"reflect"
	"testing"
)

func Test_strToBytes(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		struct {
			name string
			args args
			want []byte
		}{
			name: "strToBytes",
			args: args{
				s: "hello world",
			},
			want: []byte("hello world"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strToBytes(tt.args.s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("strToBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_bytesToStr(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		struct {
			name string
			args args
			want string
		}{
			name: "bytesToStr",
			args: args{
				b: []byte("hello world"),
			},
			want: "hello world",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bytesToStr(tt.args.b); got != tt.want {
				t.Errorf("bytesToStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_valueToBytes(t *testing.T) {
	if got := valueToBytes(DefaultCodec, fmt.Errorf("test")); !reflect.DeepEqual(got, []byte("test")) {
		t.Errorf("valueToBytes() = %v, want %v", got, []byte("test"))
	}
}

func Test_memGet(t *testing.T) {
	if got := memGet(100); len(got) != 100 {
		t.Errorf("len(memGet(100)) = %v, want %v", len(got), 100)
	}
}

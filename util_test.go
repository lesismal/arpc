// Copyright 2020 lesismal. All rights reserved.

// Use of this source code is governed by an MIT-style

// license that can be found in the LICENSE file.

package arpc

import (
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
		// TODO: Add test cases.
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
		// TODO: Add test cases.
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
	type args struct {
		codec Codec
		v     interface{}
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := valueToBytes(tt.args.codec, tt.args.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("valueToBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

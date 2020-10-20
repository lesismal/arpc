// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package util

import (
	// "fmt"
	// "runtime"
	"runtime/debug"
	"unsafe"

	acodec "github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
)

const (
	separator = "---------------------------------------\n"
)

// Empty struct
type Empty struct{}

// Recover recover panic and log stacks's info
func Recover() {
	if err := recover(); err != nil {
		log.Error("%sruntime error: %v\ntraceback:\n%v\n%v", separator, err, string(debug.Stack()), separator)
	}
}

// Safe wrap a func with panic recover
func Safe(call func()) {
	defer Recover()
	call()
}

// StrToBytes hack string to []byte
func StrToBytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

// BytesToStr hack []byte to string
func BytesToStr(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// ValueToBytes convert values to []byte
func ValueToBytes(codec acodec.Codec, v interface{}) []byte {
	if v == nil {
		return nil
	}
	var (
		err  error
		data []byte
	)
	switch vt := v.(type) {
	case []byte:
		data = vt
	case *[]byte:
		data = *vt
	case string:
		data = StrToBytes(vt)
	case *string:
		data = StrToBytes(*vt)
	case error:
		data = StrToBytes(vt.Error())
	case *error:
		data = StrToBytes((*vt).Error())
	default:
		if codec == nil {
			codec = acodec.DefaultCodec
		}
		data, err = codec.Marshal(vt)
		if err != nil {
			log.Error("ValueToBytes: %v", err)
		}
	}

	return data
}

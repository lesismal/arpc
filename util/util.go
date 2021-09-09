// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"runtime"
	"unsafe"

	acodec "github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
)

// Empty struct
type Empty struct{}

// Recover handles panic and logs stack info
func Recover() {
	if err := recover(); err != nil {
		const size = 64 << 10
		buf := make([]byte, size)
		buf = buf[:runtime.Stack(buf, false)]
		log.Error("runtime error: %v\ntraceback:\n%v\n", err, *(*string)(unsafe.Pointer(&buf)))
	}
}

// Safe wraps a function-calling with panic recovery
func Safe(call func()) {
	defer Recover()
	call()
}

// StrToBytes hacks string to []byte
func StrToBytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

// BytesToStr hacks []byte to string
func BytesToStr(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// ValueToBytes converts values to []byte
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

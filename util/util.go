// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"errors"
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

// ValueToBytesOwned converts values to []byte and reports whether the returned
// slice is safe to hold across an asynchronous send without copying.
//
// owned is true when the bytes are either freshly allocated by the codec, or
// alias an immutable string backing (string/error), both of which cannot be
// mutated by the caller after this call. owned is false only for []byte/*[]byte
// inputs, whose contents the caller may mutate; callers that keep the slice
// beyond the call (e.g. the writev queue) must copy it into their own buffer.
func ValueToBytesOwned(codec acodec.Codec, v interface{}) (data []byte, owned bool) {
	if v == nil {
		return nil, true
	}
	var err error
	switch vt := v.(type) {
	case []byte:
		return vt, false
	case *[]byte:
		return *vt, false
	case string:
		return StrToBytes(vt), true
	case *string:
		return StrToBytes(*vt), true
	case error:
		return StrToBytes(vt.Error()), true
	case *error:
		return StrToBytes((*vt).Error()), true
	default:
		if codec == nil {
			codec = acodec.DefaultCodec
		}
		data, err = codec.Marshal(vt)
		if err != nil {
			log.Error("ValueToBytesOwned: %v", err)
		}
		return data, true
	}
}

// BytesToValue converts []byte to values
func BytesToValue(codec acodec.Codec, data []byte, v interface{}) error {
	var err error
	if v != nil {
		switch vt := v.(type) {
		case *[]byte:
			*vt = make([]byte, len(data))
			copy(*vt, data)
		case *string:
			*vt = string(data)
		case *error:
			*vt = errors.New(string(data))
		default:
			if codec == nil {
				codec = acodec.DefaultCodec
			}
			err = codec.Unmarshal(data, vt)
			if err != nil {
				log.Error("ValueToBytes: %v", err)
			}
		}
	}
	return err
}

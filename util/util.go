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

// Empty is a zero-size type, mainly used as a signal channel element.
type Empty struct{}

// Recover recovers from a panic and logs it with the stack trace. It must be
// called directly by defer.
func Recover() {
	if err := recover(); err != nil {
		const size = 64 << 10
		buf := make([]byte, size)
		buf = buf[:runtime.Stack(buf, false)]
		log.Error("runtime error: %v\ntraceback:\n%v\n", err, *(*string)(unsafe.Pointer(&buf)))
	}
}

// Safe calls call and recovers from any panic it raises.
func Safe(call func()) {
	defer Recover()
	call()
}

// StrToBytes converts s to []byte without copying. The result must not be
// modified.
func StrToBytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

// BytesToStr converts b to string without copying. b must not be modified
// while the result is in use.
func BytesToStr(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// ValueToBytes converts v to []byte: []byte and string (or pointers to them)
// and error are used as-is without copying, other values are encoded by codec,
// or acodec.DefaultCodec if codec is nil. It returns nil if v is nil or the
// encoding fails.
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

// ValueToBytesOwned is like ValueToBytes, and also reports whether the result
// is safe to keep after the call returns (e.g. queued for an async send)
// without copying.
//
// owned is false only for []byte and *[]byte inputs, whose content the caller
// may still modify; callers that keep such a slice must copy it. Strings,
// errors and codec output cannot be modified by the caller, so owned is true.
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

// BytesToValue decodes data into v, which should be a pointer. *[]byte gets a
// copy of data, *string gets string(data), *error gets errors.New(string(data)),
// and other types are decoded by codec, or acodec.DefaultCodec if codec is nil.
// It does nothing if v is nil.
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

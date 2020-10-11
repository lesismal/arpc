// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"fmt"
	"runtime"
	"unsafe"
)

const (
	maxStack  = 20
	separator = "---------------------------------------\n"
)

// Empty struct
type Empty struct{}

// HandlePanic recover panic and log stacks's info
func HandlePanic() {
	if err := recover(); err != nil {
		errstr := fmt.Sprintf("%sruntime error: %v\ntraceback:\n", separator, err)

		i := 2
		for {
			pc, file, line, ok := runtime.Caller(i)
			if !ok || i > maxStack {
				break
			}
			errstr += fmt.Sprintf("    stack: %d %v [file: %s] [func: %s] [line: %d]\n", i-1, ok, file, runtime.FuncForPC(pc).Name(), line)
			i++
		}
		errstr += separator

		logError(errstr)
	}
}

// Safe wrap a closure with panic handler
func Safe(call func()) {
	defer func() {
		if err := recover(); err != nil {
			errstr := fmt.Sprintf("%sruntime error: %v\ntraceback:\n", separator, err)
			for i := 2; i <= maxStack; i++ {
				pc, file, line, ok := runtime.Caller(i)
				if !ok {
					break
				}
				errstr += fmt.Sprintf("    stack: %d %v [file: %s] [func: %s] [line: %d]\n", i-1, ok, file, runtime.FuncForPC(pc).Name(), line)
			}
			logError(errstr + separator)
		}
	}()
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
func ValueToBytes(codec Codec, v interface{}) []byte {
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
			codec = DefaultCodec
		}
		data, err = codec.Marshal(vt)
		if err != nil {
			logError("ValueToBytes: %v", err)
		}
	}

	return data
}

func memGet(size int) []byte {
	return make([]byte, size)
}

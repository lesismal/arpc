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

func safe(call func()) {
	defer func() {
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

			DefaultLogger.Error(errstr)
		}
	}()
	call()
}

func memGet(size int) []byte {
	return make([]byte, size)
}

func strToBytes(s string) []byte {
	// return []byte(s)
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func bytesToStr(b []byte) string {
	// return string(b)
	return *(*string)(unsafe.Pointer(&b))
}

func valueToBytes(codec Codec, v interface{}) []byte {
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
		data = strToBytes(vt)
	case *string:
		data = strToBytes(*vt)
	case error:
		data = strToBytes(vt.Error())
	case *error:
		data = strToBytes((*vt).Error())
	default:
		if codec == nil {
			codec = DefaultCodec
		}
		data, err = codec.Marshal(vt)
		if err != nil {
			logError("valueToBytes: %v", err)
		}
	}

	return data
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/lesismal/arpc"
)

const (
	maxStack  = 20
	separator = "---------------------------------------\n"
)

// // GetLogger .
// func GetLogger() arpc.Logger {
// 	return arpc.DefaultLogger
// }

// // SetLogger .
// func SetLogger(l arpc.Logger) {
// 	arpc.DefaultLogger = l
// }

type empty struct{}

func handlePanic() {
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

		arpc.DefaultLogger.Error(errstr)
	}
}

func safe(call func()) {
	defer handlePanic()
	call()
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

func valueToBytes(codec arpc.Codec, v interface{}) []byte {
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
			codec = arpc.DefaultCodec
		}
		data, err = codec.Marshal(vt)
		if err != nil {
			panic(err)
		}
	}

	return data
}

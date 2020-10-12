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
	maxStack  = 20
	separator = "---------------------------------------\n"
)

// Empty struct
type Empty struct{}

// Recover() recover panic and log stacks's info
func Recover() {
	if err := recover(); err != nil {
		log.Error("%sruntime error: %v\ntraceback:\n%v\n\v", separator, err, string(debug.Stack()), separator)

		// errstr := fmt.Sprintf("%sruntime error: %v\ntraceback:\n", separator, err)
		// i := 2
		// for {
		// 	pc, file, line, ok := runtime.Caller(i)
		// 	if !ok || i > maxStack {
		// 		break
		// 	}
		// 	errstr += fmt.Sprintf("    stack: %d %v [file: %s] [func: %s] [line: %d]\n", i-1, ok, file, runtime.FuncForPC(pc).Name(), line)
		// 	i++
		// }
		// errstr += separator

		// log.Error(errstr)
	}
}

// Safe wrap a closure with panic handler
func Safe(call func()) {
	defer func() {
		if err := recover(); err != nil {
			log.Error("%sruntime error: %v\ntraceback:\n%v\n\v", separator, err, string(debug.Stack()), separator)

			// errstr := fmt.Sprintf("%sruntime error: %v\ntraceback:\n", separator, err)
			// for i := 2; i <= maxStack; i++ {
			// 	pc, file, line, ok := runtime.Caller(i)
			// 	if !ok {
			// 		break
			// 	}
			// 	errstr += fmt.Sprintf("    stack: %d %v [file: %s] [func: %s] [line: %d]\n", i-1, ok, file, runtime.FuncForPC(pc).Name(), line)
			// }
			// log.Error(errstr + separator)
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

func GetBuffer(size int) []byte {
	return make([]byte, size)
}

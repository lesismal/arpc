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

		DefaultLogger.Error(errstr)
	}
}

func safe(call func()) {
	defer handlePanic()
	call()
}

func strToBytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func bytesToStr(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func valueToBytes(codec Codec, v interface{}) []byte {
	var (
		err  error
		data []byte
	)
	if v != nil {
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
			data, err = codec.Marshal(vt)
			if err != nil {
				panic(err)
			}
		}
	}

	return data
}

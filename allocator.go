// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

type Allocator interface {
	Malloc(size int) []byte
	Realloc(buf []byte, size int) []byte
	Append(buf []byte, more ...byte) []byte
	AppendString(buf []byte, more string) []byte
	Free(buf []byte)
}

// DefaultAllocator .
var DefaultAllocator Allocator = New(64, 64)

// BufferPool .
type BufferPool struct {
	Debug bool
	mux   sync.Mutex

	smallSize int
	bigSize   int
	smallPool *sync.Pool
	bigPool   *sync.Pool

	allocCnt    uint64
	freeCnt     uint64
	allocStacks map[uintptr]string
}

// New .
func New(smallSize, bigSize int) Allocator {
	if smallSize <= 0 {
		smallSize = 64
	}
	if bigSize <= 0 {
		bigSize = 64 * 1024
	}
	if bigSize < smallSize {
		bigSize = smallSize
	}

	bp := &BufferPool{
		smallSize:   smallSize,
		bigSize:     bigSize,
		allocStacks: map[uintptr]string{},
		smallPool:   &sync.Pool{},
		bigPool:     &sync.Pool{},
		// Debug:       true,
	}
	bp.smallPool.New = func() interface{} {
		buf := make([]byte, smallSize)
		return &buf
	}
	bp.bigPool.New = func() interface{} {
		buf := make([]byte, bigSize)
		return &buf
	}
	if bigSize == smallSize {
		bp.bigPool = bp.smallPool
	}

	return bp
}

// Malloc .
func (bp *BufferPool) Malloc(size int) []byte {
	pool := bp.smallPool
	if size >= bp.bigSize {
		pool = bp.bigPool
	}

	pbuf := pool.Get().(*[]byte)
	need := size - cap(*pbuf)
	if need > 0 {
		*pbuf = append((*pbuf)[:cap(*pbuf)], make([]byte, need)...)
	}

	if bp.Debug {
		bp.mux.Lock()
		defer bp.mux.Unlock()
		ptr := getBufferPtr(*pbuf)
		bp.addAllocStack(ptr)
	}

	return (*pbuf)[:size]
}

// Realloc .
func (bp *BufferPool) Realloc(buf []byte, size int) []byte {
	if size <= cap(buf) {
		return buf[:size]
	}

	if !bp.Debug {
		if cap(buf) < bp.bigSize && size >= bp.bigSize {
			pbuf := bp.bigPool.Get().(*[]byte)
			need := size - cap(*pbuf)
			if need > 0 {
				*pbuf = append((*pbuf)[:cap(*pbuf)], make([]byte, need)...)
			}
			*pbuf = (*pbuf)[:size]
			copy(*pbuf, buf)
			bp.Free(buf)
			return *pbuf
		}
		need := size - cap(buf)
		if need > 0 {
			buf = append(buf[:cap(buf)], make([]byte, need)...)
		}
		return buf[:size]
	}

	return bp.reallocDebug(buf, size)
}

func (bp *BufferPool) reallocDebug(buf []byte, size int) []byte {
	if cap(buf) == 0 {
		panic("realloc zero size buf")
	}
	if cap(buf) < bp.bigSize && size >= bp.bigSize {
		pbuf := bp.bigPool.Get().(*[]byte)
		need := size - cap(*pbuf)
		if need > 0 {
			*pbuf = append((*pbuf)[:cap(*pbuf)], make([]byte, need)...)
		}
		*pbuf = (*pbuf)[:size]
		copy(*pbuf, buf)
		bp.Free(buf)
		ptr := getBufferPtr(*pbuf)
		bp.mux.Lock()
		defer bp.mux.Unlock()
		bp.addAllocStack(ptr)
		return *pbuf
	}
	oldPtr := getBufferPtr(buf)
	need := size - cap(buf)
	if need > 0 {
		buf = append(buf[:cap(buf)], make([]byte, need)...)
	}
	newPtr := getBufferPtr(buf)
	if newPtr != oldPtr {
		bp.mux.Lock()
		defer bp.mux.Unlock()
		bp.deleteAllocStack(oldPtr)
		bp.addAllocStack(newPtr)
	}

	return (buf)[:size]
}

// Append .
func (bp *BufferPool) Append(buf []byte, more ...byte) []byte {
	return bp.AppendString(buf, *(*string)(unsafe.Pointer(&more)))
}

// AppendString .
func (bp *BufferPool) AppendString(buf []byte, more string) []byte {
	if !bp.Debug {
		bl := len(buf)
		total := bl + len(more)
		if bl < bp.bigSize && total >= bp.bigSize {
			pbuf := bp.bigPool.Get().(*[]byte)
			need := total - cap(*pbuf)
			if need > 0 {
				*pbuf = append((*pbuf)[:cap(*pbuf)], make([]byte, need)...)
			}
			*pbuf = (*pbuf)[:total]
			copy(*pbuf, buf)
			copy((*pbuf)[bl:], more)
			bp.Free(buf)
			return *pbuf
		}
		return append(buf, more...)
	}
	return bp.appendStringDebug(buf, more)
}

func (bp *BufferPool) appendStringDebug(buf []byte, more string) []byte {
	if cap(buf) == 0 {
		panic("append zero cap buf")
	}
	bl := len(buf)
	total := bl + len(more)
	if bl < bp.bigSize && total >= bp.bigSize {
		pbuf := bp.bigPool.Get().(*[]byte)
		need := total - cap(*pbuf)
		if need > 0 {
			*pbuf = append((*pbuf)[:cap(*pbuf)], make([]byte, need)...)
		}
		*pbuf = (*pbuf)[:total]
		copy(*pbuf, buf)
		copy((*pbuf)[bl:], more)
		bp.Free(buf)
		ptr := getBufferPtr(*pbuf)
		bp.mux.Lock()
		defer bp.mux.Unlock()
		bp.addAllocStack(ptr)
		return *pbuf
	}

	oldPtr := getBufferPtr(buf)
	buf = append(buf, more...)
	newPtr := getBufferPtr(buf)
	if newPtr != oldPtr {
		bp.mux.Lock()
		defer bp.mux.Unlock()
		bp.deleteAllocStack(oldPtr)
		bp.addAllocStack(newPtr)
	}
	return buf
}

// Free .
func (bp *BufferPool) Free(buf []byte) {
	size := cap(buf)
	pool := bp.smallPool
	if size >= bp.bigSize {
		pool = bp.bigPool
	}

	if bp.Debug {
		bp.mux.Lock()
		defer bp.mux.Unlock()
		ptr := getBufferPtr(buf)
		bp.deleteAllocStack(ptr)
	}

	pool.Put(&buf)
}

func (bp *BufferPool) addAllocStack(ptr uintptr) {
	bp.allocCnt++
	bp.allocStacks[ptr] = getStack()
}

func (bp *BufferPool) deleteAllocStack(ptr uintptr) {
	if _, ok := bp.allocStacks[ptr]; !ok {
		panic("delete buffer which is not from pool")
	}
	bp.freeCnt++
	delete(bp.allocStacks, ptr)
}

func (bp *BufferPool) LogDebugInfo() {
	bp.mux.Lock()
	defer bp.mux.Unlock()
	fmt.Println("---------------------------------------------------------")
	fmt.Println("BufferPool Debug Info:")
	fmt.Println("---------------------------------------------------------")
	for ptr, stack := range bp.allocStacks {
		fmt.Println("ptr:", ptr)
		fmt.Println("stack:\n", stack)
		fmt.Println("---------------------------------------------------------")
	}
	// fmt.Println("---------------------------------------------------------")
	// fmt.Println("Free")
	// for s, n := range bp.freeStacks {
	// 	fmt.Println("num:", n)
	// 	fmt.Println("stack:\n", s)
	// 	totalFree += n
	// 	fmt.Println("---------------------------------------------------------")
	// }
	fmt.Println("Alloc Without Free:", bp.allocCnt-bp.freeCnt)
	fmt.Println("TotalAlloc        :", bp.allocCnt)
	fmt.Println("TotalFree         :", bp.freeCnt)
	fmt.Println("---------------------------------------------------------")
}

// NativeAllocator definition.
type NativeAllocator struct{}

// Malloc .
func (a *NativeAllocator) Malloc(size int) []byte {
	return make([]byte, size)
}

// Realloc .
func (a *NativeAllocator) Realloc(buf []byte, size int) []byte {
	if size <= cap(buf) {
		return buf[:size]
	}
	newBuf := make([]byte, size)
	copy(newBuf, buf)
	return newBuf
}

// Free .
func (a *NativeAllocator) Free(buf []byte) {
}

// Malloc exports default package method.
func Malloc(size int) []byte {
	return DefaultAllocator.Malloc(size)
}

// Realloc exports default package method.
func Realloc(buf []byte, size int) []byte {
	return DefaultAllocator.Realloc(buf, size)
}

// Append exports default package method.
func Append(buf []byte, more ...byte) []byte {
	return DefaultAllocator.Append(buf, more...)
}

// AppendString exports default package method.
func AppendString(buf []byte, more string) []byte {
	return DefaultAllocator.AppendString(buf, more)
}

// Free exports default package method.
func Free(buf []byte) {
	DefaultAllocator.Free(buf)
}

// SetDebug .
func SetDebug(enable bool) {
	bp, ok := DefaultAllocator.(*BufferPool)
	if ok {
		bp.Debug = enable
	}
}

// LogDebugInfo .
func LogDebugInfo() {
	bp, ok := DefaultAllocator.(*BufferPool)
	if ok {
		bp.LogDebugInfo()
	}
}

func getBufferPtr(buf []byte) uintptr {
	if cap(buf) == 0 {
		panic("zero cap buffer")
	}
	return uintptr(unsafe.Pointer(&((buf)[:1][0])))
}

func getStack() string {
	i := 2
	str := ""
	for ; i < 10; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		str += fmt.Sprintf("\tstack: %d %v [file: %s] [func: %s] [line: %d]\n", i-1, ok, file, runtime.FuncForPC(pc).Name(), line)
	}
	return str
}

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

// Allocator allocates and recycles byte buffers.
type Allocator interface {
	// Malloc returns a buffer of length size.
	Malloc(size int) []byte
	// Realloc resizes buf to size, keeping its content. The result may be a
	// new buffer, in which case buf must not be used any more.
	Realloc(buf []byte, size int) []byte
	// Append appends more to buf, like the builtin append.
	Append(buf []byte, more ...byte) []byte
	// AppendString appends more to buf, like the builtin append.
	AppendString(buf []byte, more string) []byte
	// Free recycles buf, which must not be used after that.
	Free(buf []byte)
}

// DefaultAllocator is the package-level Allocator: a BufferPool with a single
// pool of 64-byte-initial buffers.
var DefaultAllocator Allocator = New(64, 64)

// BufferPool is an Allocator backed by two sync.Pools: one for buffers smaller
// than bigSize and one for the others. Buffers grow as needed and are reused
// with their grown capacity.
type BufferPool struct {
	// Debug enables leak tracking: the stack of each Malloc is recorded until
	// the buffer is freed, and freeing an untracked buffer panics.
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

// New creates a BufferPool. Buffers with a size >= bigSize come from the big
// pool, others from the small pool. smallSize defaults to 64, bigSize to 64KB,
// and bigSize is at least smallSize; if they are equal, one pool is used.
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

// Malloc returns a pooled buffer of length size, growing it if its capacity
// is not enough.
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

// Realloc resizes buf to size. When buf grows from below bigSize to bigSize
// or more, it moves to a buffer from the big pool and the old one is freed.
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

// reallocDebug is Realloc with alloc stack tracking.
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

// Append appends more to buf, see AppendString.
func (bp *BufferPool) Append(buf []byte, more ...byte) []byte {
	return bp.AppendString(buf, *(*string)(unsafe.Pointer(&more)))
}

// AppendString appends more to buf. When the length grows from below bigSize
// to bigSize or more, the result moves to a buffer from the big pool and buf
// is freed.
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

// appendStringDebug is AppendString with alloc stack tracking.
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

// Free puts buf back to the pool chosen by its capacity.
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

// addAllocStack records the caller stack of the buffer at ptr; bp.mux must be
// held.
func (bp *BufferPool) addAllocStack(ptr uintptr) {
	bp.allocCnt++
	bp.allocStacks[ptr] = getStack()
}

// deleteAllocStack drops the record of the buffer at ptr, and panics if there
// is none; bp.mux must be held.
func (bp *BufferPool) deleteAllocStack(ptr uintptr) {
	if _, ok := bp.allocStacks[ptr]; !ok {
		panic("delete buffer which is not from pool")
	}
	bp.freeCnt++
	delete(bp.allocStacks, ptr)
}

// LogDebugInfo prints the stacks of the buffers not freed yet and the
// alloc/free counts to stdout. Only meaningful with Debug enabled.
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

// NativeAllocator allocates with make and lets the GC recycle buffers. It does
// not implement Append and AppendString, so it is not a full Allocator.
type NativeAllocator struct{}

// Malloc returns make([]byte, size).
func (a *NativeAllocator) Malloc(size int) []byte {
	return make([]byte, size)
}

// Realloc returns buf resliced to size if its capacity is enough, otherwise a
// new buffer with buf's content.
func (a *NativeAllocator) Realloc(buf []byte, size int) []byte {
	if size <= cap(buf) {
		return buf[:size]
	}
	newBuf := make([]byte, size)
	copy(newBuf, buf)
	return newBuf
}

// Free does nothing.
func (a *NativeAllocator) Free(buf []byte) {
}

// Malloc calls DefaultAllocator.Malloc.
func Malloc(size int) []byte {
	return DefaultAllocator.Malloc(size)
}

// Realloc calls DefaultAllocator.Realloc.
func Realloc(buf []byte, size int) []byte {
	return DefaultAllocator.Realloc(buf, size)
}

// Append calls DefaultAllocator.Append.
func Append(buf []byte, more ...byte) []byte {
	return DefaultAllocator.Append(buf, more...)
}

// AppendString calls DefaultAllocator.AppendString.
func AppendString(buf []byte, more string) []byte {
	return DefaultAllocator.AppendString(buf, more)
}

// Free calls DefaultAllocator.Free.
func Free(buf []byte) {
	DefaultAllocator.Free(buf)
}

// SetDebug sets Debug of DefaultAllocator if it is a *BufferPool.
func SetDebug(enable bool) {
	bp, ok := DefaultAllocator.(*BufferPool)
	if ok {
		bp.Debug = enable
	}
}

// LogDebugInfo calls LogDebugInfo of DefaultAllocator if it is a *BufferPool.
func LogDebugInfo() {
	bp, ok := DefaultAllocator.(*BufferPool)
	if ok {
		bp.LogDebugInfo()
	}
}

// getBufferPtr returns the address of buf's first element, which identifies
// the buffer in debug mode. It panics on a zero-capacity buffer.
func getBufferPtr(buf []byte) uintptr {
	if cap(buf) == 0 {
		panic("zero cap buffer")
	}
	return uintptr(unsafe.Pointer(&((buf)[:1][0])))
}

// getStack returns up to 8 frames of the caller's caller stack.
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

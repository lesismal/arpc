package arpc

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// DefaultAllocator .
var DefaultAllocator = New(1024, 1024*1024*1024)

type Allocator interface {
	Malloc(size int) *[]byte
	Realloc(buf *[]byte, size int) *[]byte // deprecated.
	Append(buf *[]byte, more ...byte) *[]byte
	AppendString(buf *[]byte, more string) *[]byte
	Free(buf *[]byte)
}

type DebugAllocator interface {
	Allocator
	String() string
	SetDebug(bool)
}

//go:norace
func Malloc(size int) *[]byte {
	return DefaultAllocator.Malloc(size)
}

//go:norace
func Realloc(pbuf *[]byte, size int) *[]byte {
	return DefaultAllocator.Realloc(pbuf, size)
}

//go:norace
func Append(pbuf *[]byte, more ...byte) *[]byte {
	return DefaultAllocator.Append(pbuf, more...)
}

//go:norace
func AppendString(pbuf *[]byte, more string) *[]byte {
	return DefaultAllocator.AppendString(pbuf, more)
}

//go:norace
func Free(pbuf *[]byte) {
	DefaultAllocator.Free(pbuf)
}

type sizeMap struct {
	MallocCount int64 `json:"MallocCount"`
	FreeCount   int64 `json:"FreeCount"`
	NeedFree    int64 `json:"NeedFree"`
}

type debugger struct {
	mux         sync.Mutex
	on          bool
	MallocCount int64            `json:"MallocCount"`
	FreeCount   int64            `json:"FreeCount"`
	NeedFree    int64            `json:"NeedFree"`
	SizeMap     map[int]*sizeMap `json:"SizeMap"`
}

//go:norace
func (d *debugger) SetDebug(dbg bool) {
	d.on = dbg
}

//go:norace
func (d *debugger) incrMalloc(pbuf *[]byte) {
	if d.on {
		d.incrMallocSlow(pbuf)
	}
}

//go:norace
func (d *debugger) incrMallocSlow(pbuf *[]byte) {
	atomic.AddInt64(&d.MallocCount, 1)
	atomic.AddInt64(&d.NeedFree, 1)
	size := cap(*pbuf)
	d.mux.Lock()
	defer d.mux.Unlock()
	if d.SizeMap == nil {
		d.SizeMap = map[int]*sizeMap{}
	}
	if v, ok := d.SizeMap[size]; ok {
		v.MallocCount++
		v.NeedFree++
	} else {
		d.SizeMap[size] = &sizeMap{
			MallocCount: 1,
			NeedFree:    1,
		}
	}
}

//go:norace
func (d *debugger) incrFree(pbuf *[]byte) {
	if d.on {
		d.incrFreeSlow(pbuf)
	}
}

//go:norace
func (d *debugger) incrFreeSlow(pbuf *[]byte) {
	atomic.AddInt64(&d.FreeCount, 1)
	atomic.AddInt64(&d.NeedFree, -1)
	size := cap(*pbuf)
	d.mux.Lock()
	defer d.mux.Unlock()
	if v, ok := d.SizeMap[size]; ok {
		v.FreeCount++
		v.NeedFree--
	} else {
		d.SizeMap[size] = &sizeMap{
			MallocCount: 1,
			NeedFree:    -1,
		}
	}
}

//go:norace
func (d *debugger) String() string {
	if d.on {
		b, err := json.Marshal(d)
		if err == nil {
			return string(b)
		}
	}
	return ""
}

// MemPool .
type MemPool struct {
	*debugger
	bufSize  int
	freeSize int
	pool     *sync.Pool
}

// New .
func New(bufSize, freeSize int) Allocator {
	if bufSize <= 0 {
		bufSize = 64
	}
	if freeSize <= 0 {
		freeSize = 64 * 1024
	}
	if freeSize < bufSize {
		freeSize = bufSize
	}

	mp := &MemPool{
		debugger: &debugger{},
		bufSize:  bufSize,
		freeSize: freeSize,
		pool:     &sync.Pool{},
		// Debug:       true,
	}
	mp.pool.New = func() interface{} {
		buf := make([]byte, bufSize)
		return &buf
	}
	return mp
}

// Malloc .
func (mp *MemPool) Malloc(size int) *[]byte {
	var ret []byte
	if size > mp.freeSize {
		ret = make([]byte, size)
		mp.incrMalloc(&ret)
		return &ret
	}
	pbuf := mp.pool.Get().(*[]byte)
	n := cap(*pbuf)
	if n < size {
		*pbuf = append((*pbuf)[:n], make([]byte, size-n)...)
	}
	(*pbuf) = (*pbuf)[:size]
	mp.incrMalloc(pbuf)
	return pbuf
}

// Realloc .
func (mp *MemPool) Realloc(pbuf *[]byte, size int) *[]byte {
	if size <= cap(*pbuf) {
		*pbuf = (*pbuf)[:size]
		return pbuf
	}

	if cap(*pbuf) < mp.freeSize {
		newBufPtr := mp.pool.Get().(*[]byte)
		n := cap(*newBufPtr)
		if n < size {
			*newBufPtr = append((*newBufPtr)[:n], make([]byte, size-n)...)
		}
		*newBufPtr = (*newBufPtr)[:size]
		copy(*newBufPtr, *pbuf)
		mp.Free(pbuf)
		return newBufPtr
	}
	*pbuf = append((*pbuf)[:cap(*pbuf)], make([]byte, size-cap(*pbuf))...)[:size]
	return pbuf
}

// Append .
func (mp *MemPool) Append(pbuf *[]byte, more ...byte) *[]byte {
	*pbuf = append(*pbuf, more...)
	return pbuf
}

// AppendString .
func (mp *MemPool) AppendString(pbuf *[]byte, more string) *[]byte {
	*pbuf = append(*pbuf, more...)
	return pbuf
}

// Free .
func (mp *MemPool) Free(pbuf *[]byte) {
	if pbuf != nil && cap(*pbuf) > 0 {
		mp.incrFree(pbuf)
		if cap(*pbuf) > mp.freeSize {
			return
		}
		mp.pool.Put(pbuf)
	}
}

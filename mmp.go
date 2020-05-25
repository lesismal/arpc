// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"errors"
	"sync"
)

var pos = []byte{0, 1, 28, 2, 29, 14, 24, 3,
	30, 22, 20, 15, 25, 17, 4, 8, 31, 27, 13, 23, 21, 19,
	16, 7, 26, 12, 18, 6, 11, 5, 10, 9}

// MemPool definition
type MemPool struct {
	maxSize int
	buffers []sync.Pool
}

// debrujin algorithm
func (pool *MemPool) definition(size int) byte {
	v := uint32(size)
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v = (v >> 1) + 1
	ret := pos[(v*0x077CB531)>>27]
	if size > 1<<ret {
		ret++
	}
	return ret
}

// init buffers
func (pool *MemPool) init(maxSize int) {
	pool.maxSize = maxSize

	pool.buffers = make([]sync.Pool, pool.definition(maxSize)+1)
	for k := range pool.buffers {
		i := k
		pool.buffers[k].New = func() interface{} {
			return make([]byte, 1<<uint32(i))
		}
	}
}

// Get borrows []byte from pool
func (pool *MemPool) Get(size int) []byte {
	if size <= 0 || size > pool.maxSize {
		return nil
	}
	return pool.buffers[pool.definition(size)].Get().([]byte)[:size]
}

// Put payback []byte to pool
func (pool *MemPool) Put(buf []byte) error {
	bits := pool.definition(cap(buf))
	if cap(buf) == 0 || cap(buf) > pool.maxSize || cap(buf) != 1<<bits {
		return errors.New("MemPool Put() incorrect buffer size")
	}
	pool.buffers[bits].Put(buf)
	return nil
}

// New factory
func newMemPool(maxSize int) *MemPool {
	pool := &MemPool{}
	pool.init(maxSize)
	return pool
}

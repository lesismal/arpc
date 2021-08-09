// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"sync"
)

// BufferPool is the default buffer pool instance.
var BufferPool = &bufferPool{MinSize: 64}

// ChosMemPool
type bufferPool struct {
	sync.Pool
	MinSize int
}

func init() {
	BufferPool.Pool.New = func() interface{} {
		return make([]byte, BufferPool.MinSize)
	}
}

// Malloc returns a buffer with size.
func (p *bufferPool) Malloc(size int) []byte {
	buf := p.Get().([]byte)
	if cap(buf) < size {
		if cap(buf) >= p.MinSize {
			p.Put(buf)
		}
		buf = make([]byte, size)
	}

	return buf[:size]
}

// Free put the buffer back to the pool.
func (p *bufferPool) Free(buf []byte) {
	if cap(buf) < p.MinSize {
		return
	}
	p.Put(buf)
}

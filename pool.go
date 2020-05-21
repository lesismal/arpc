// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"sync"

	"github.com/lesismal/mmp"
)

var (
	// Mem Pool
	memPool = mmp.New(MaxBodyLen)

	// Context Pool
	ctxPool = sync.Pool{
		New: func() interface{} {
			return &Context{}
		},
	}

	// Client Pool
	clientPool = sync.Pool{
		New: func() interface{} {
			return &Client{}
		},
	}
)

func memGet(size int) []byte {
	return memPool.Get(size)
}

func memPut(b []byte) {
	memPool.Put(b)
}

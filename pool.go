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
)

func memGet(size int) []byte {
	return memPool.Get(size)
}

func memPut(b []byte) {
	memPool.Put(b)
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	// "fmt"
	"sync"
	"sync/atomic"
	// "time"

	"github.com/lesismal/mmp"
)

var (
	// Mem Pool
	memGetCnt = uint64(0)
	memPutCnt = uint64(0)
	memPool   = mmp.New(MaxBodyLen)

	// Context Pool
	ctxGetCnt = uint64(0)
	ctxPutCnt = uint64(0)
	ctxPool   = sync.Pool{
		New: func() interface{} {
			return &Context{}
		},
	}

	// Message Pool
	msgGetCnt = uint64(0)
	msgPutCnt = uint64(0)
	msgPool   = sync.Pool{
		New: func() interface{} {
			return &Message{}
		},
	}

	// rpcSession Pool
	sessionGetCnt = uint64(0)
	sessionPutCnt = uint64(0)
	sessionPool   = sync.Pool{
		New: func() interface{} {
			return &rpcSession{done: make(chan *Message, 1)}
		},
	}
)

func init() {
	// go func() {
	// 	for {
	// 		time.Sleep(time.Second)
	// 		fmt.Println("111 msgGetCnt:", atomic.LoadUint64(&msgGetCnt))
	// 		fmt.Println("222 msgPutCnt:", atomic.LoadUint64(&msgPutCnt))
	// 		fmt.Println("333 ctxGetCnt:", atomic.LoadUint64(&msgGetCnt))
	// 		fmt.Println("444 ctxPutCnt:", atomic.LoadUint64(&msgPutCnt))
	// 	}
	// }()
}

func ctxGet() *Context {
	atomic.AddUint64(&ctxGetCnt, 1)
	return ctxPool.Get().(*Context)
}

func ctxPut(ctx *Context) {
	if ctx != nil {
		atomic.AddUint64(&ctxPutCnt, 1)
		ctx.Client = nil
		ctx.Message = nil
		ctx.done = false
		ctx.index = -1
		ctx.handlers = nil
		ctxPool.Put(ctx)
	}
}

func msgGet(size int) *Message {
	atomic.AddUint64(&msgGetCnt, 1)
	msg := msgPool.Get().(*Message)
	msg.Buffer = memGet(size)
	msg.isBroadcast = false
	msg.ref = 0
	return msg
}

func msgPut(msg *Message) {
	if msg != nil {
		atomic.AddUint64(&msgPutCnt, 1)
		msg.values = nil
		msgPool.Put(msg)
	}
}

func memGet(size int) []byte {
	atomic.AddUint64(&memGetCnt, 1)
	return memPool.Get(size)
}

func memPut(b []byte) {
	atomic.AddUint64(&memPutCnt, 1)
	memPool.Put(b)
}

func sessionGet(seq uint64) *rpcSession {
	atomic.AddUint64(&sessionGetCnt, 1)
	sess := sessionPool.Get().(*rpcSession)
	sess.seq = seq
	return sess
}

func sessionPut(sess *rpcSession) {
	if sess != nil {
		atomic.AddUint64(&sessionPutCnt, 1)
		select {
		case msg := <-sess.done:
			msgPut(msg)
		default:
		}
		sessionPool.Put(sess)
	}
}

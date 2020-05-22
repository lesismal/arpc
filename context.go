// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"encoding/binary"
	"errors"
	"time"
)

// Context definition
type Context struct {
	Client  *Client
	Message Message
}

// Body returns body
func (ctx *Context) Body() []byte {
	return ctx.Message.Body()
}

// Bind parses data to struct
func (ctx *Context) Bind(v interface{}) error {
	if v != nil {
		data := ctx.Message.Body()
		switch vt := v.(type) {
		case *[]byte:
			*vt = data
		case *string:
			*vt = string(data)
		case *error:
			*vt = errors.New(bytesToStr(data))
		default:
			return ctx.Client.Codec.Unmarshal(data, v)
		}
	}
	return nil
}

// Write responses message to client
func (ctx *Context) Write(v interface{}) error {
	return ctx.write(RPCCmdRsp, v, TimeForever)
}

// WriteWithTimeout responses message to client with timeout
func (ctx *Context) WriteWithTimeout(v interface{}, timeout time.Duration) error {
	return ctx.write(RPCCmdRsp, v, timeout)
}

// Error responses error message to client
func (ctx *Context) Error(err interface{}) error {
	return ctx.write(RPCCmdErr, err, TimeForever)
}

// Clone a new Contex, new Context's lifecycle depends on user, should call Contex.Release after Contex.write
func (ctx *Context) Clone() *Context {
	return ctxGet(ctx.Client, ctx.Message)
}

// Release payback Contex to pool
func (ctx *Context) Release() {
	ctxPool.Put(ctx)
}

func (ctx *Context) newRspMessage(cmd byte, v interface{}) Message {
	var (
		data    []byte
		msg     Message
		bodyLen int
	)

	data = valueToBytes(ctx.Client.Codec, v)

	bodyLen = len(data)
	msg = Message(memGet(HeadLen + bodyLen))
	binary.LittleEndian.PutUint32(msg[headerIndexBodyLenBegin:headerIndexBodyLenEnd], uint32(bodyLen))
	binary.LittleEndian.PutUint64(msg[headerIndexSeqBegin:headerIndexSeqEnd], ctx.Message.Seq())
	msg[headerIndexCmd] = cmd
	msg[headerIndexAsync] = ctx.Message.Async()
	msg[headerIndexMethodLen] = 0
	copy(msg[HeadLen:], data)

	return msg
}

func (ctx *Context) write(cmd byte, v interface{}, timeout time.Duration) error {
	msg := ctx.newRspMessage(cmd, v)
	return ctx.Client.PushMsg(msg, timeout)
}

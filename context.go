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
	if !ctx.Message.IsRef() {
		return ctx.Message.Body()
	}
	return nil
}

// Bind body data to struct
func (ctx *Context) Bind(v interface{}) error {
	msg := ctx.Message
	if msg.IsRef() {
		panic(ErrBindClonedContex)
	}
	if msg.IsError() {
		return msg.Error()
	}
	if v != nil {
		data := msg.Body()
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
	return ctx.write(v, 0, TimeForever)
}

// WriteWithTimeout responses message to client with timeout
func (ctx *Context) WriteWithTimeout(v interface{}, timeout time.Duration) error {
	return ctx.write(v, 0, timeout)
}

// Error responses error message to client
func (ctx *Context) Error(err error) error {
	return ctx.write(err, 1, TimeForever)
}

// Clone a new Contex, new Context's lifecycle depends on user, should call Contex.Release after Contex.write
func (ctx *Context) Clone() *Context {
	return ctxGet(ctx.Client, ctx.Message.cloneHead())
}

// Release payback Contex to pool
func (ctx *Context) Release() {
	if ctx.Message.IsRef() {
		memPut(ctx.Message)
	}
	ctxPut(ctx)
}

func (ctx *Context) newRspMessage(cmd byte, v interface{}, isError byte) Message {
	var (
		data    []byte
		msg     Message
		bodyLen int
		realMsg = ctx.Message.Payload()
	)

	if _, ok := v.(error); ok {
		isError = 1
	}

	data = valueToBytes(ctx.Client.Codec, v)

	bodyLen = len(data)
	msg = Message(memGet(HeadLen + bodyLen))
	binary.LittleEndian.PutUint32(msg[headerIndexBodyLenBegin:headerIndexBodyLenEnd], uint32(bodyLen))
	binary.LittleEndian.PutUint64(msg[headerIndexSeqBegin:headerIndexSeqEnd], realMsg.Seq())
	msg[headerIndexCmd] = cmd
	msg[headerIndexAsync] = realMsg.Async()
	msg[headerIndexError] = isError
	msg[headerIndexMethodLen] = 0
	copy(msg[HeadLen:], data)

	return msg
}

func (ctx *Context) write(v interface{}, isError byte, timeout time.Duration) error {
	if ctx.Message.Payload().Cmd() != CmdRequest {
		panic(ErrResponseToResponsedMessage)
	}
	msg := ctx.newRspMessage(CmdResponse, v, isError)
	return ctx.Client.PushMsg(msg, timeout)
}

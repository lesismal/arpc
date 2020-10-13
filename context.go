// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/lesismal/arpc/util"
)

// Context definition
type Context struct {
	Client  *Client
	Message Message

	done     bool
	index    int
	handlers []HandlerFunc
}

// Body returns body
func (ctx *Context) Body() []byte {
	return ctx.Message.Data()
}

// Bind body data to struct
func (ctx *Context) Bind(v interface{}) error {
	msg := ctx.Message
	if msg.IsError() {
		return msg.Error()
	}
	if v != nil {
		data := msg.Data()
		switch vt := v.(type) {
		case *[]byte:
			*vt = data
		case *string:
			*vt = string(data)
		case *error:
			*vt = errors.New(util.BytesToStr(data))
		default:
			return ctx.Client.Codec.Unmarshal(data, v)
		}
	}
	return nil
}

// Write responses message to client
func (ctx *Context) Write(v interface{}) error {
	return ctx.write(v, false, TimeForever)
}

// WriteWithTimeout responses message to client with timeout
func (ctx *Context) WriteWithTimeout(v interface{}, timeout time.Duration) error {
	return ctx.write(v, false, timeout)
}

// Error responses error message to client
func (ctx *Context) Error(v interface{}) error {
	return ctx.write(v, true, TimeForever)
}

// Next .
func (ctx *Context) Next() {
	ctx.index++
	if !ctx.done && ctx.index < len(ctx.handlers) {
		ctx.handlers[ctx.index](ctx)
	}
	// for !ctx.done && ctx.index < len(ctx.handlers) {
	// 	ctx.handlers[ctx.index](ctx)
	// 	ctx.index++
	// }
}

// Done .
func (ctx *Context) Done() {
	ctx.done = true
}

func (ctx *Context) newRspMessage(v interface{}, isError bool) Message {
	var (
		data      []byte
		msg       Message
		bodyLen   int
		methodLen int
	)

	if _, ok := v.(error); ok {
		isError = true
	}

	data = util.ValueToBytes(ctx.Client.Codec, v)

	methodLen = ctx.Message.MethodLen()
	bodyLen = len(data) + methodLen
	msg = Message(ctx.Client.Handler.GetBuffer(HeadLen + bodyLen))
	copy(msg[HeaderIndexFlag:], ctx.Message[HeaderIndexFlag:HeadLen+methodLen])
	binary.LittleEndian.PutUint32(msg[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(bodyLen))
	// binary.LittleEndian.PutUint64(msg[HeaderIndexSeqBegin:HeaderIndexSeqEnd], ctx.Message.Seq())
	msg[HeaderIndexCmd] = CmdResponse
	msg.SetError(isError)
	copy(msg[HeadLen+methodLen:], data)

	return msg
}

func (ctx *Context) write(v interface{}, isError bool, timeout time.Duration) error {
	msg := ctx.newRspMessage(v, isError)
	return ctx.Client.PushMsg(msg, timeout)
}

func newContext(c *Client, msg Message, handlers []HandlerFunc) *Context {
	return &Context{Client: c, Message: msg, done: false, index: -1, handlers: handlers}
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"errors"
	"time"

	"github.com/lesismal/arpc/util"
)

// Context definition
type Context struct {
	Client  *Client
	Message Message

	err      interface{}
	response []interface{}
	timeout  time.Duration

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

func (ctx *Context) write(v interface{}, isError bool, timeout time.Duration) error {
	if ctx.err != nil {
		return ErrContextErrWritten
	}
	if ctx.Message.Cmd() != CmdRequest {
		return ErrContextResponseToNotify
	}
	if _, ok := v.(error); ok {
		isError = true
	}
	if isError {
		ctx.err = v
	} else {
		ctx.response = append(ctx.response, v)
	}
	ctx.timeout = timeout
	return nil
}

func (ctx *Context) Dump() error {
	cli := ctx.Client
	req := ctx.Message
	var rsp Message
	if ctx.err != nil {
		rsp = newMessage(CmdResponse, req.method(), ctx.err, true, req.IsAsync(), req.Seq(), cli.Handler, cli.Codec)
	} else {
		var data []byte
		for _, v := range ctx.response {
			data = append(data, util.ValueToBytes(cli.Codec, v)...)
		}
		rsp = newMessage(CmdResponse, req.method(), data, false, req.IsAsync(), req.Seq(), cli.Handler, cli.Codec)
	}

	return ctx.Client.PushMsg(rsp, ctx.timeout)
}

func newContext(c *Client, msg Message, handlers []HandlerFunc) *Context {
	return &Context{Client: c, Message: msg, done: false, index: -1, handlers: handlers}
}

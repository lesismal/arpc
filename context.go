// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"math"
	"sync"
	"time"
)

var (
	contextPool = sync.Pool{
		New: func() interface{} {
			return &Context{}
		},
	}

	emptyContext = Context{}
)

// Context represents an arpc Call's context.
type Context struct {
	Client  *Client
	Message *Message

	index    int
	handlers []HandlerFunc
}

func (ctx *Context) Release() {
	ctx.Message.Release()
	*ctx = emptyContext
	contextPool.Put(ctx)
}

// Get returns value for key.
func (ctx *Context) Get(key interface{}) (interface{}, bool) {
	if len(ctx.Message.values) == 0 {
		return nil, false
	}
	value, ok := ctx.Message.values[key]
	return value, ok
}

// Set sets key-value pair.
func (ctx *Context) Set(key interface{}, value interface{}) {
	if key == nil || value == nil {
		return
	}
	if ctx.Message.values == nil {
		ctx.Message.values = map[interface{}]interface{}{}
	}
	ctx.Message.values[key] = value
}

// Values returns values.
func (ctx *Context) Values() map[interface{}]interface{} {
	if ctx.Message == nil {
		return nil
	}
	return ctx.Message.values
}

// Body returns body.
func (ctx *Context) Body() []byte {
	return ctx.Message.Data()
}

// Bind parses the body data and stores the result
// in the value pointed to by v.
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
		// case *error:
		// 	*vt = errors.New(util.BytesToStr(data))
		default:
			return ctx.Client.Codec.Unmarshal(data, v)
		}
	}
	return nil
}

// Write responses a Message to the Client.
func (ctx *Context) Write(v interface{}) error {
	return ctx.write(v, false, TimeForever)
}

// WriteWithTimeout responses a Message to the Client with timeout.
func (ctx *Context) WriteWithTimeout(v interface{}, timeout time.Duration) error {
	return ctx.write(v, false, timeout)
}

// Error responses an error Message to the Client.
func (ctx *Context) Error(v interface{}) error {
	return ctx.write(v, v != nil, TimeForever)
}

// Next calls next middleware or method/router handler.
func (ctx *Context) Next() {
	index := int(ctx.index)
	if index < len(ctx.handlers) {
		ctx.index++
		ctx.handlers[index](ctx)
	}
}

// Abort stops the one-by-one-calling of middlewares and method/router handler.
func (ctx *Context) Abort() {
	ctx.index = int(math.MaxInt8)
}

// Deadline implements stdlib's Context.
func (ctx *Context) Deadline() (deadline time.Time, ok bool) {
	return
}

// Done implements stdlib's Context.
func (ctx *Context) Done() <-chan struct{} {
	return nil
}

// Err implements stdlib's Context.
func (ctx *Context) Err() error {
	return nil
}

// Value returns the value associated with this context for key, implements stdlib's Context.
func (ctx *Context) Value(key interface{}) interface{} {
	value, _ := ctx.Get(key)
	return value
}

func (ctx *Context) write(v interface{}, isError bool, timeout time.Duration) error {
	cli := ctx.Client
	if !cli.Handler.AsyncWrite() {
		return ctx.writeDirectly(v, isError)
	}
	req := ctx.Message
	if req.Cmd() != CmdRequest {
		return ErrContextResponseToNotify
	}
	if _, ok := v.(error); ok {
		isError = true
	}
	rsp := newMessage(CmdResponse, req.method(), v, isError, req.IsAsync(), req.Seq(), cli.Handler, cli.Codec, ctx.Message.values)
	return cli.PushMsg(rsp, timeout)
}

func (ctx *Context) writeDirectly(v interface{}, isError bool) error {
	cli := ctx.Client
	req := ctx.Message
	if req.Cmd() != CmdRequest {
		return ErrContextResponseToNotify
	}
	if _, ok := v.(error); ok {
		isError = true
	}
	rsp := newMessage(CmdResponse, req.method(), v, isError, req.IsAsync(), req.Seq(), cli.Handler, cli.Codec, ctx.Message.values)
	if !cli.reconnecting {
		coders := cli.Handler.Coders()
		for j := 0; j < len(coders); j++ {
			rsp = coders[j].Encode(cli, rsp)
		}
		_, err := cli.Handler.Send(cli.Conn, rsp.Buffer)
		if err != nil {
			cli.Conn.Close()
		}
		return err
	}
	cli.dropMessage(rsp)
	return ErrClientReconnecting
}

func newContext(cli *Client, msg *Message, handlers []HandlerFunc) *Context {
	ctx := contextPool.Get().(*Context)
	ctx.Client = cli
	ctx.Message = msg
	ctx.Message.values = msg.values
	ctx.handlers = handlers
	return ctx
}

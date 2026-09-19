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

// Context carries an incoming Message and the Client it came from through the
// middleware and handler chain. It also implements context.Context, though it
// never expires or gets canceled.
//
// Contexts are taken from a pool, but only go back to it via Release, which
// is usually called from the Handler.HandleContextDone callback.
type Context struct {
	// Client is the connection the Message was received from.
	Client *Client
	// Message is the incoming Message.
	Message *Message

	index       int
	handlers    []HandlerFunc
	responseErr interface{}
}

// Release releases the Message and puts the Context back to the pool. The
// Context must not be used after that.
func (ctx *Context) Release() {
	ctx.Message.Release()
	*ctx = emptyContext
	contextPool.Put(ctx)
}

// ResponseError returns the error value written by Error, or by Write with an
// error value. It is only recorded when the Handler uses AsyncWrite, and is
// nil otherwise.
func (ctx *Context) ResponseError() interface{} {
	return ctx.responseErr
}

// Get returns the value stored in the Message's values for key.
func (ctx *Context) Get(key interface{}) (interface{}, bool) {
	if len(ctx.Message.values) == 0 {
		return nil, false
	}
	value, ok := ctx.Message.values[key]
	return value, ok
}

// Set stores a key-value pair in the Message's values. It does nothing if key
// or value is nil.
func (ctx *Context) Set(key interface{}, value interface{}) {
	if key == nil || value == nil {
		return
	}
	if ctx.Message.values == nil {
		ctx.Message.values = map[interface{}]interface{}{}
	}
	ctx.Message.values[key] = value
}

// Values returns the Message's values, or nil if there is no Message.
func (ctx *Context) Values() map[interface{}]interface{} {
	if ctx.Message == nil {
		return nil
	}
	return ctx.Message.values
}

// Body returns the Message body.
func (ctx *Context) Body() []byte {
	return ctx.Message.Data()
}

// Bind decodes the Message body into v. If the Message is an error response,
// it returns that error instead.
//
// *[]byte is set to the body without copying, so it is only valid until the
// Message is released; *string gets a copy; other types are decoded by the
// Client's Codec.
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

// Write sends v as the response to the request. If v is an error, the
// response is an error response. It returns ErrContextResponseToNotify if the
// incoming Message is not a request.
func (ctx *Context) Write(v interface{}) error {
	return ctx.write(v, false, TimeForever)
}

// WriteWithTimeout is like Write, with a timeout for pushing the response
// into the send queue. The timeout only applies when the Handler uses
// AsyncWrite.
func (ctx *Context) WriteWithTimeout(v interface{}, timeout time.Duration) error {
	return ctx.write(v, false, timeout)
}

// Error sends v as an error response to the request. If v is nil, a normal
// response with an empty body is sent.
func (ctx *Context) Error(v interface{}) error {
	return ctx.write(v, v != nil, TimeForever)
}

// Next runs the rest of the handler chain. The chain continues on its own
// after each middleware returns, so a middleware only needs to call Next to
// run code after the rest of the chain, e.g. to measure the handling time.
func (ctx *Context) Next() {
	index := int(ctx.index)
	if index < len(ctx.handlers) {
		ctx.index++
		ctx.handlers[index](ctx)
	}
}

// Abort stops the rest of the handler chain from being called.
func (ctx *Context) Abort() {
	ctx.index = math.MaxInt
}

// Deadline implements context.Context. It never has a deadline.
func (ctx *Context) Deadline() (deadline time.Time, ok bool) {
	return
}

// Done implements context.Context. It returns nil: the Context is never canceled.
func (ctx *Context) Done() <-chan struct{} {
	return nil
}

// Err implements context.Context. It always returns nil.
func (ctx *Context) Err() error {
	return nil
}

// Value implements context.Context by returning Get(key).
func (ctx *Context) Value(key interface{}) interface{} {
	value, _ := ctx.Get(key)
	return value
}

// write builds the response to the request and pushes it into the send queue,
// or sends it directly if the Handler does not use AsyncWrite. An error value
// always makes an error response.
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
	if isError {
		ctx.responseErr = v
	}

	rsp := newMessage(CmdResponse, req.method(), v, isError, req.IsAsync(), req.Seq(), cli.Handler, cli.Codec, ctx.Message.values)
	return cli.PushMsg(rsp, timeout)
}

// writeDirectly encodes and writes the response on the calling goroutine. The
// connection is closed on a write error, and the response is dropped while the
// Client is reconnecting.
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

// newContext gets a Context from the pool for msg and its handler chain.
func newContext(cli *Client, msg *Message, handlers []HandlerFunc) *Context {
	ctx := contextPool.Get().(*Context)
	ctx.Client = cli
	ctx.Message = msg
	ctx.Message.values = msg.values
	ctx.handlers = handlers
	return ctx
}

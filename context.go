// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"time"
)

// Context represents an arpc Call's context.
type Context struct {
	Client  *Client
	Message *Message
	values  map[string]interface{}

	err      interface{}
	response []interface{}
	timeout  time.Duration

	done     bool
	index    int
	handlers []HandlerFunc
}

// Get returns value for key.
func (ctx *Context) Get(key string) (interface{}, bool) {
	if len(ctx.values) == 0 {
		return nil, false
	}
	value, ok := ctx.values[key]
	return value, ok
}

// Set sets key-value pair.
func (ctx *Context) Set(key string, value interface{}) {
	if value == nil {
		return
	}
	if ctx.values == nil {
		ctx.values = map[string]interface{}{}
	}
	ctx.values[key] = value
}

// Values returns values.
func (ctx *Context) Values() map[string]interface{} {
	return ctx.values
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
	return ctx.write(v, true, TimeForever)
}

// Next calls next middleware or method/router handler.
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

// Abort stops the one-by-one-calling of middlewares and method/router handler.
func (ctx *Context) Abort() {
	ctx.done = true
}

// Deadline returns the time when work done on behalf of this context
// should be canceled. Deadline returns ok==false when no deadline is
// set. Successive calls to Deadline return the same results.
func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return
}

// Done returns a channel that's closed when work done on behalf of this
// context should be canceled. Done may return nil if this context can
// never be canceled. Successive calls to Done return the same value.
func (c *Context) Done() <-chan struct{} {
	return nil
}

// Err returns a non-nil error value after Done is closed,
// successive calls to Err return the same error.
// If Done is not yet closed, Err returns nil.
// If Done is closed, Err returns a non-nil error explaining why:
// Canceled if the context was canceled
// or DeadlineExceeded if the context's deadline passed.
func (c *Context) Err() error {
	return nil
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
func (c *Context) Value(key interface{}) interface{} {
	if keyString, ok := key.(string); ok {
		value, _ := c.Get(keyString)
		return value
	}
	return nil
}

func (ctx *Context) write(v interface{}, isError bool, timeout time.Duration) error {
	cli := ctx.Client
	req := ctx.Message
	if req.Cmd() != CmdRequest {
		return ErrContextResponseToNotify
	}
	if _, ok := v.(error); ok {
		isError = true
	}
	rsp := newMessage(CmdResponse, req.method(), v, isError, req.IsAsync(), req.Seq(), cli.Handler, cli.Codec, ctx.values)
	return cli.PushMsg(rsp, ctx.timeout)
}

func newContext(cli *Client, msg *Message, handlers []HandlerFunc) *Context {
	return &Context{Client: cli, Message: msg, values: msg.values, done: false, index: -1, handlers: handlers}
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

// DefaultHandler is the default Handler used by arpc
var DefaultHandler Handler = NewHandler()

// HandlerFunc defines message handler of arpc middleware and method/router.
type HandlerFunc func(*Context)

// routerHandler saves all middleware and method/router handler funcs
// for every method by register order,
// all the funcs will be called one by one for every message.
type routerHandler struct {
	async    bool
	handlers []HandlerFunc
}

// Handler defines net message handler interface.
type Handler interface {
	// Clone returns a copy of Handler.
	Clone() Handler

	// LogTag returns log tag value.
	LogTag() string
	// SetLogTag sets log tag.
	SetLogTag(tag string)

	// HandleConnected registers handler which will be called when client connected.
	HandleConnected(onConnected func(*Client))
	// OnConnected will be called when client is connected.
	OnConnected(c *Client)

	// HandleDisconnected registers handler which will be called when client is disconnected.
	HandleDisconnected(onDisConnected func(*Client))
	// OnDisconnected will be called when client is disconnected.
	OnDisconnected(c *Client)

	// MaxReconnectTimes returns client's max reconnect times.
	MaxReconnectTimes() int
	// SetMaxReconnectTimes sets client's max reconnect times for.
	SetMaxReconnectTimes(n int)

	// HandleOverstock registers handler which will be called when client send queue is overstock.
	HandleOverstock(onOverstock func(c *Client, m *Message))
	// OnOverstock will be called when client chSend is full.
	OnOverstock(c *Client, m *Message)

	// HandleMessageDone registers handler which will be called when message dropped.
	HandleMessageDone(onMessageDone func(c *Client, m *Message))
	// OnMessageDone will be called when message is dropped.
	OnMessageDone(c *Client, m *Message)

	// HandleMessageDropped registers handler which will be called when message dropped.
	HandleMessageDropped(onOverstock func(c *Client, m *Message))
	// OnOverstock will be called when message is dropped.
	OnMessageDropped(c *Client, m *Message)

	// HandleSessionMiss registers handler which will be called when async message seq not found.
	HandleSessionMiss(onSessionMiss func(c *Client, m *Message))
	// OnSessionMiss will be called when async message seq not found.
	OnSessionMiss(c *Client, m *Message)

	// HandleContextDone registers handler which will be called when message dropped.
	HandleContextDone(onContextDone func(ctx *Context))
	// OnContextDone will be called when message is dropped.
	OnContextDone(ctx *Context)

	// BeforeRecv registers handler which will be called before Recv.
	BeforeRecv(h func(net.Conn) error)
	// BeforeRecvHandler returns the handler which will be called before Recv.
	BeforeRecvHandler() func(net.Conn) error
	// BeforeSend registers handler which will be called before Send.
	BeforeSend(h func(net.Conn) error)
	// BeforeSendHandler returns the handler which will be called before Send.
	BeforeSendHandler() func(net.Conn) error

	// BatchRecv returns BatchRecv flag.
	BatchRecv() bool
	// SetBatchRecv sets BatchRecv flag.
	SetBatchRecv(batch bool)
	// BatchSend returns BatchSend flag.
	BatchSend() bool
	// SetBatchSend sets BatchSend flag.
	SetBatchSend(batch bool)

	// AsyncWrite returns AsyncWrite flag.
	AsyncWrite() bool
	// SetAsyncWrite sets AsyncWrite flag.
	SetAsyncWrite(async bool)

	// AsyncResponse returns AsyncResponse flag.
	AsyncResponse() bool
	// SetAsyncResponse sets AsyncResponse flag.
	SetAsyncResponse(async bool)

	// WrapReader wraps net.Conn to Read data with io.Reader.
	WrapReader(conn net.Conn) io.Reader
	// SetReaderWrapper registers reader wrapper for net.Conn.
	SetReaderWrapper(wrapper func(conn net.Conn) io.Reader)

	// Recv reads a message from a client.
	Recv(c *Client) (*Message, error)
	// Send writes buffer data to a connection.
	Send(c net.Conn, writer *bufio.Writer, buffer []byte) (int, error)
	// // SendN writes multiple buffer data to a connection.
	// SendN(conn net.Conn, buffers net.Buffers) (int, error)

	// RecvBufferSize returns client's recv buffer size.
	RecvBufferSize() int
	// SetRecvBufferSize sets client's recv buffer size.
	SetRecvBufferSize(size int)

	// SendBufferSize returns client's send buffer size.
	SendBufferSize() int
	// SetSendBufferSize sets client's send buffer size.
	SetSendBufferSize(size int)

	// ReadTimeout returns client's read timeout.
	ReadTimeout() time.Duration
	// SetReadTimeout sets client's read timeout.
	SetReadTimeout(timeout time.Duration)

	// WriteTimeout returns client's write timeout.
	WriteTimeout() time.Duration
	// SetWriteTimeout sets client's write timeout.
	SetWriteTimeout(timeout time.Duration)

	// SendQueueSize returns client's send queue channel capacity.
	SendQueueSize() int
	// SetSendQueueSize sets client's send queue channel capacity.
	SetSendQueueSize(size int)

	// MaxBodyLen returns max body length of a message.
	MaxBodyLen() int
	// SetMaxBodyLen sets max body length of a message.
	SetMaxBodyLen(l int)

	// Use registers method/router handler middleware.
	Use(h HandlerFunc)

	// UseCoder registers message coding middleware,
	// coder.Encode will be called before message send,
	// coder.Decode will be called after message recv.
	UseCoder(coder MessageCoder)

	// Coders returns coding middlewares.
	Coders() []MessageCoder

	// Handle registers method/router handler.
	//
	// If pass a Boolean value of "true", the handler will be called asynchronously in a new goroutine,
	// Else the handler will be called synchronously in the client's reading goroutine one by one.
	Handle(m string, h HandlerFunc, args ...interface{})

	// HandleNotFound registers "" method/router handler,
	// It will be called when mothod/router is not found.
	HandleNotFound(h HandlerFunc)

	// OnMessage finds method/router middlewares and handler, then call them one by one.
	OnMessage(c *Client, m *Message)

	// Malloc makes a buffer by size.
	Malloc(size int) []byte
	// HandleMalloc registers buffer maker.
	HandleMalloc(f func(size int) []byte)

	// Append append bytes to buffer.
	Append(b []byte, more ...byte) []byte
	// HandleAppend registers buffer appender.
	HandleAppend(f func(b []byte, more ...byte) []byte)

	// Free release a buffer.
	Free([]byte)
	// HandleFree registers buffer releaser.
	HandleFree(f func(buf []byte))

	// EnablePool registers handlers for pool operation for Context and Message and Message.Buffer
	EnablePool(enable bool)

	Context() (context.Context, context.CancelFunc)
	SetContext(ctx context.Context, cancel context.CancelFunc)
	Cancel()

	// NewMessage creates a Message.
	NewMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, codec codec.Codec, values map[interface{}]interface{}) *Message

	// NewMessageWithBuffer creates a message with the buffer and manage the message by the pool.
	// The buffer arg should be managed by a pool if EnablePool(true) .
	NewMessageWithBuffer(buffer []byte) *Message

	// SetAsyncExecutor sets executor.
	SetAsyncExecutor(executor func(f func()))
	// AsyncExecute executes a func
	AsyncExecute(f func())
}

// handler represents a default Handler implementation.
type handler struct {
	logtag            string
	batchRecv         bool
	batchSend         bool
	asyncWrite        bool
	asyncResponse     bool
	recvBufferSize    int
	sendBufferSize    int
	readTimeout       time.Duration
	writeTimeout      time.Duration
	sendQueueSize     int
	maxBodyLen        int
	maxReconnectTimes int

	onConnected      func(*Client)
	onDisConnected   func(*Client)
	onOverstock      func(c *Client, m *Message)
	onMessageDone    func(c *Client, m *Message)
	onMessageDropped func(c *Client, m *Message)
	onSessionMiss    func(c *Client, m *Message)
	onContextDone    func(ctx *Context)

	beforeRecv func(net.Conn) error
	beforeSend func(net.Conn) error
	malloc     func(int) []byte
	append     func([]byte, ...byte) []byte
	free       func([]byte)

	wrapReader func(conn net.Conn) io.Reader

	middles   []HandlerFunc
	msgCoders []MessageCoder

	routes map[string]*routerHandler

	ctx    context.Context
	cancel context.CancelFunc

	executor func(f func())
}

func (h *handler) Clone() Handler {
	cp := *h
	cp.middles = make([]HandlerFunc, len(h.middles))
	copy(cp.middles, h.middles)

	cp.msgCoders = make([]MessageCoder, len(h.msgCoders))
	copy(cp.msgCoders, h.msgCoders)

	cp.routes = map[string]*routerHandler{}
	for k, v := range h.routes {
		rh := &routerHandler{
			async:    v.async,
			handlers: make([]HandlerFunc, len(v.handlers)),
		}
		copy(rh.handlers, v.handlers)
		cp.routes[k] = rh
	}

	ctx, cancel := context.WithCancel(context.Background())
	cp.ctx = ctx
	cp.cancel = cancel

	return &cp
}

func (h *handler) LogTag() string {
	return h.logtag
}

func (h *handler) SetLogTag(tag string) {
	h.logtag = tag
}

func (h *handler) HandleConnected(onConnected func(*Client)) {
	if onConnected == nil {
		return
	}
	pre := h.onConnected
	h.onConnected = func(c *Client) {
		if pre != nil {
			pre(c)
		}
		onConnected(c)
	}
}

func (h *handler) OnConnected(c *Client) {
	if h.onConnected != nil {
		h.onConnected(c)
	}
}

func (h *handler) HandleDisconnected(onDisConnected func(*Client)) {
	if onDisConnected == nil {
		return
	}
	pre := h.onDisConnected
	h.onDisConnected = func(c *Client) {
		if pre != nil {
			pre(c)
		}
		onDisConnected(c)
	}
}

func (h *handler) OnDisconnected(c *Client) {
	if h.onDisConnected != nil {
		h.onDisConnected(c)
	}
}

func (h *handler) MaxReconnectTimes() int {
	return h.maxReconnectTimes
}

func (h *handler) SetMaxReconnectTimes(n int) {
	h.maxReconnectTimes = n
}

func (h *handler) HandleOverstock(onOverstock func(c *Client, m *Message)) {
	h.onOverstock = func(c *Client, m *Message) {
		onOverstock(c, m)
		h.OnMessageDone(c, m)
	}
}

func (h *handler) OnOverstock(c *Client, m *Message) {
	if h.onOverstock != nil {
		h.onOverstock(c, m)
	}
}

func (h *handler) HandleMessageDropped(onMessageDropped func(c *Client, m *Message)) {
	h.onMessageDropped = func(c *Client, m *Message) {
		onMessageDropped(c, m)
		h.OnMessageDone(c, m)
	}
}

func (h *handler) OnMessageDropped(c *Client, m *Message) {
	if h.onMessageDropped != nil {
		h.onMessageDropped(c, m)
	}
}

func (h *handler) HandleMessageDone(onMessageDone func(c *Client, m *Message)) {
	h.onMessageDone = onMessageDone
}

func (h *handler) OnMessageDone(c *Client, m *Message) {
	if h.onMessageDone != nil && m != nil {
		h.onMessageDone(c, m)
	}
}

func (h *handler) HandleSessionMiss(onSessionMiss func(c *Client, m *Message)) {
	h.onSessionMiss = onSessionMiss
}

func (h *handler) OnSessionMiss(c *Client, m *Message) {
	if h.onSessionMiss != nil {
		h.onSessionMiss(c, m)
		h.OnMessageDone(c, m)
	}
}

func (h *handler) HandleContextDone(onContextDone func(ctx *Context)) {
	h.onContextDone = onContextDone
}

func (h *handler) OnContextDone(ctx *Context) {
	if h.onContextDone != nil {
		h.onContextDone(ctx)
	}
}

func (h *handler) BeforeRecv(hb func(net.Conn) error) {
	h.beforeRecv = hb
}

func (h *handler) BeforeRecvHandler() func(net.Conn) error {
	return h.beforeRecv
}

func (h *handler) BeforeSend(hs func(net.Conn) error) {
	h.beforeSend = hs
}

func (h *handler) BeforeSendHandler() func(net.Conn) error {
	return h.beforeSend
}

func (h *handler) BatchRecv() bool {
	return h.batchRecv
}

func (h *handler) SetBatchRecv(batch bool) {
	h.batchRecv = batch
}

func (h *handler) BatchSend() bool {
	return h.batchSend
}

func (h *handler) SetBatchSend(batch bool) {
	h.batchSend = batch
}

func (h *handler) AsyncWrite() bool {
	return h.asyncWrite
}

func (h *handler) SetAsyncWrite(async bool) {
	h.asyncWrite = async
}

func (h *handler) AsyncResponse() bool {
	return h.asyncResponse
}

func (h *handler) SetAsyncResponse(async bool) {
	h.asyncResponse = async
}

func (h *handler) WrapReader(conn net.Conn) io.Reader {
	if h.wrapReader != nil {
		return h.wrapReader(conn)
	}
	return conn
}

func (h *handler) SetReaderWrapper(wrapper func(conn net.Conn) io.Reader) {
	h.wrapReader = wrapper
}

func (h *handler) RecvBufferSize() int {
	return h.recvBufferSize
}

func (h *handler) SetRecvBufferSize(size int) {
	h.recvBufferSize = size
}

func (h *handler) SendBufferSize() int {
	return h.sendBufferSize
}

func (h *handler) SetSendBufferSize(size int) {
	h.sendBufferSize = size
}

func (h *handler) ReadTimeout() time.Duration {
	return h.readTimeout
}

func (h *handler) SetReadTimeout(timeout time.Duration) {
	h.readTimeout = timeout
}

func (h *handler) WriteTimeout() time.Duration {
	return h.writeTimeout
}

func (h *handler) SetWriteTimeout(timeout time.Duration) {
	h.writeTimeout = timeout
}

func (h *handler) SendQueueSize() int {
	return h.sendQueueSize
}

func (h *handler) SetSendQueueSize(size int) {
	h.sendQueueSize = size
}

func (h *handler) MaxBodyLen() int {
	return h.maxBodyLen
}

func (h *handler) SetMaxBodyLen(l int) {
	h.maxBodyLen = l
}

func (h *handler) Use(cb HandlerFunc) {
	if cb == nil {
		return
	}
	cbWithNext := func(ctx *Context) {
		cb(ctx)
		ctx.Next()
	}
	h.middles = append(h.middles, cbWithNext)
	for k, v := range h.routes {
		rh := &routerHandler{
			async:    v.async,
			handlers: make([]HandlerFunc, len(v.handlers)+1),
		}
		copy(rh.handlers, v.handlers)
		rh.handlers[len(v.handlers)] = cbWithNext
		h.routes[k] = rh
	}
}

func (h *handler) UseCoder(coder MessageCoder) {
	if coder != nil {
		h.msgCoders = append(h.msgCoders, coder)
	}
}

func (h *handler) Coders() []MessageCoder {
	return h.msgCoders
}

func (h *handler) Handle(method string, cb HandlerFunc, args ...interface{}) {
	if method == "" {
		panic(fmt.Errorf("empty('') method is reserved for [method not found], should use HandleNotFound to register '' handler"))
	}
	h.handle(method, cb, args...)
}

func (h *handler) HandleNotFound(cb HandlerFunc) {
	h.handle("", cb)
}

func (h *handler) handle(method string, cb HandlerFunc, args ...interface{}) {
	if h.routes == nil {
		h.routes = map[string]*routerHandler{}
	}
	if len(method) > MaxMethodLen {
		panic(fmt.Errorf("invalid method length %v(> MaxMethodLen %v)", len(method), MaxMethodLen))
	}

	if _, ok := h.routes[""]; !ok {
		rh := &routerHandler{
			async:    false,
			handlers: make([]HandlerFunc, len(h.middles)+1),
		}
		copy(rh.handlers, h.middles)
		rh.handlers[len(h.middles)] = func(ctx *Context) {
			ctx.Error(ErrMethodNotFound)
			ctx.Next()
		}
		h.routes[""] = rh
	}

	if _, ok := h.routes[method]; ok && method != "" {
		panic(fmt.Errorf("handler exist for method %v ", method))
	}

	async := h.AsyncResponse()
	if len(args) > 0 {
		if bv, ok := args[0].(bool); ok {
			async = bv
		}
	}
	rh := &routerHandler{
		async:    async,
		handlers: make([]HandlerFunc, len(h.middles)+1),
	}
	copy(rh.handlers, h.middles)
	rh.handlers[len(h.middles)] = func(ctx *Context) {
		cb(ctx)
		ctx.Next()
	}
	h.routes[method] = rh
}

func (h *handler) Recv(c *Client) (*Message, error) {
	var (
		err     error
		message *Message
	)

	if h.beforeRecv != nil {
		if err = h.beforeRecv(c.Conn); err != nil {
			return nil, err
		}
	}
	if h.readTimeout > 0 {
		c.Conn.SetReadDeadline(time.Now().Add(h.readTimeout))
	}

	_, err = io.ReadFull(c.Reader, c.Head[:])
	if err != nil {
		return nil, err
	}

	message, err = c.Head.message(h)
	if err != nil {
		return nil, err
	}

	if message.Len() >= HeadLen {
		_, err = io.ReadFull(c.Reader, message.Buffer[HeaderIndexBodyLenEnd:])
	}

	return message, err
}

func (h *handler) Send(conn net.Conn, writer *bufio.Writer, buffer []byte) (int, error) {
	n, err := writer.Write(buffer)
	return n, err
}

// func (h *handler) SendN(conn net.Conn, buffers net.Buffers) (int, error) {
// 	if h.beforeSend != nil {
// 		if err := h.beforeSend(conn); err != nil {
// 			return -1, err
// 		}
// 	}
// 	if h.writeTimeout > 0 {
// 		conn.SetWriteDeadline(time.Now().Add(h.writeTimeout))
// 	}

// 	n64, err := buffers.WriteTo(conn)
// 	return int(n64), err
// }

func (h *handler) OnMessage(c *Client, msg *Message) {
	defer util.Recover()

	switch msg.Cmd() {
	case CmdPing:
		c.Pong()
		return
	case CmdPong:
		return
	}

	for i := len(h.msgCoders) - 1; i >= 0; i-- {
		msg = h.msgCoders[i].Decode(c, msg)
	}

	ml := msg.MethodLen()
	if ml <= 0 || ml > MaxMethodLen || ml > (msg.Len()-HeadLen) {
		log.Warn("%v OnMessage: invalid request method length %v, dropped", h.LogTag(), ml)
		return
	}

	cmd := msg.Cmd()
	switch cmd {
	case CmdRequest, CmdNotify:
		method := msg.method()
		if rh, ok := h.routes[method]; ok {
			ctx := newContext(c, msg, rh.handlers)
			if !rh.async {
				ctx.Next()
				h.OnContextDone(ctx)
			} else {
				h.AsyncExecute(func() {
					ctx.Next()
					h.OnContextDone(ctx)
				})
			}
		} else {
			if rh, ok = h.routes[""]; ok {
				ctx := newContext(c, msg, rh.handlers)
				ctx.Next()
				h.OnContextDone(ctx)
			}

			if cmd == CmdRequest {
				log.Warn("%v OnMessage: invalid Call with method: [%v], no handler", h.LogTag(), method)
			} else {
				log.Warn("%v OnMessage: invalid Notify with method: [%v], no handler", h.LogTag(), method)
			}
		}
	case CmdResponse:
		if !msg.IsAsync() {
			seq := msg.Seq()
			session, ok := c.getSession(seq)
			if ok {
				session.done <- msg
			} else {
				h.OnSessionMiss(c, msg)
				log.Warn("%v OnMessage: session not exist or expired", h.LogTag())
			}
		} else {
			ah, ok := c.getAndDeleteAsyncHandler(msg.Seq())
			if ok {
				ctx := newContext(c, msg, nil)
				if ah.t != nil {
					ah.t.Stop()
				}
				ah.h(ctx)
				h.OnContextDone(ctx)
			} else {
				h.OnSessionMiss(c, msg)
				log.Warn("%v OnMessage: async handler not exist or expired", h.LogTag())
			}
		}
	default:
		log.Warn("%v OnMessage: invalid cmd [%v]", h.LogTag(), msg.Cmd())
		go c.Stop()
	}
}

func (h *handler) Malloc(size int) []byte {
	if h.malloc != nil {
		return h.malloc(size)
	}
	return make([]byte, size)
}

func (h *handler) HandleMalloc(f func(int) []byte) {
	h.malloc = f
}

func (h *handler) Append(b []byte, more ...byte) []byte {
	if h.append != nil {
		return h.append(b, more...)
	}
	return append(b, more...)
}

func (h *handler) HandleAppend(f func(b []byte, more ...byte) []byte) {
	h.append = f
}

func (h *handler) Free(b []byte) {
	if h.free != nil {
		h.free(b)
	}
}

func (h *handler) HandleFree(f func([]byte)) {
	h.free = f
}

func (h *handler) EnablePool(enable bool) {
	if enable {
		h.HandleMalloc(DefaultAllocator.Malloc)
		h.HandleAppend(DefaultAllocator.Append)
		h.HandleFree(DefaultAllocator.Free)
		h.HandleContextDone(func(ctx *Context) {
			ctx.Release()
		})
		h.HandleMessageDone(func(c *Client, m *Message) {
			m.Release()
		})
	} else {
		h.HandleMalloc(func(size int) []byte {
			return make([]byte, size)
		})
		h.HandleAppend(func(b []byte, more ...byte) []byte {
			return append(b, more...)
		})
		h.HandleFree(func(buf []byte) {})
		h.HandleContextDone(func(ctx *Context) {})
		h.HandleMessageDone(func(c *Client, m *Message) {})
	}
}

func (h *handler) Context() (context.Context, context.CancelFunc) {
	return h.ctx, h.cancel
}

func (h *handler) SetContext(ctx context.Context, cancel context.CancelFunc) {
	h.ctx = ctx
	h.cancel = cancel
}

func (h *handler) Cancel() {
	if h.cancel != nil {
		h.cancel()
	}
}

func (h *handler) NewMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, codec codec.Codec, values map[interface{}]interface{}) *Message {
	return newMessage(cmd, method, v, false, false, seq, h, codec, values)
}

func (h *handler) NewMessageWithBuffer(buffer []byte) *Message {
	msg := messagePool.Get().(*Message)
	msg.Buffer = buffer
	msg.handler = h
	return msg
}

// SetAsyncExecutor sets executor for message.
func (h *handler) SetAsyncExecutor(executor func(f func())) {
	h.executor = executor
}

// AsyncExecute executes a func.
func (h *handler) AsyncExecute(f func()) {
	if h.executor != nil {
		h.executor(f)
	} else {
		go util.Safe(f)
	}
}

// NewHandler returns a default Handler implementation.
func NewHandler() Handler {
	h := &handler{
		logtag:         "[ARPC CLI]",
		batchRecv:      true,
		batchSend:      true,
		asyncWrite:     true,
		asyncResponse:  true,
		recvBufferSize: 8192,
		sendQueueSize:  4096,
		maxBodyLen:     DefaultMaxBodyLen,
	}
	h.wrapReader = func(conn net.Conn) io.Reader {
		return bufio.NewReaderSize(conn, h.recvBufferSize)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.ctx = ctx
	h.cancel = cancel
	return h
}

// SetHandler sets default Handler.
func SetHandler(h Handler) {
	DefaultHandler = h
}

// SetLogTag sets DefaultHandler's log tag.
func SetLogTag(tag string) {
	DefaultHandler.SetLogTag(tag)
}

// HandleConnected registers default handler which will be called when client connected.
func HandleConnected(onConnected func(*Client)) {
	DefaultHandler.HandleConnected(onConnected)
}

// HandleDisconnected registers default handler which will be called when client disconnected.
func HandleDisconnected(onDisConnected func(*Client)) {
	DefaultHandler.HandleDisconnected(onDisConnected)
}

// HandleOverstock registers default handler which will be called when client send queue is overstock.
func HandleOverstock(onOverstock func(c *Client, m *Message)) {
	DefaultHandler.HandleOverstock(onOverstock)
}

// HandleMessageDropped registers default handler which will be called when message dropped.
func HandleMessageDropped(onOverstock func(c *Client, m *Message)) {
	DefaultHandler.HandleMessageDropped(onOverstock)
}

// HandleSessionMiss registers default handler which will be called when async message seq not found.
func HandleSessionMiss(onSessionMiss func(c *Client, m *Message)) {
	DefaultHandler.HandleSessionMiss(onSessionMiss)
}

// BeforeRecv registers default handler which will be called before Recv.
func BeforeRecv(h func(net.Conn) error) {
	DefaultHandler.BeforeRecv(h)
}

// BeforeSend registers default handler which will be called before Send.
func BeforeSend(h func(net.Conn) error) {
	DefaultHandler.BeforeSend(h)
}

// BatchRecv returns default BatchRecv flag.
func BatchRecv() bool {
	return DefaultHandler.BatchRecv()
}

// SetBatchRecv sets default BatchRecv flag.
func SetBatchRecv(batch bool) {
	DefaultHandler.SetBatchRecv(batch)
}

// BatchSend returns default BatchSend flag.
func BatchSend() bool {
	return DefaultHandler.BatchSend()
}

// SetBatchSend sets default BatchSend flag.
func SetBatchSend(batch bool) {
	DefaultHandler.SetBatchSend(batch)
}

// AsyncResponse returns default AsyncResponse flag.
func AsyncResponse() bool {
	return DefaultHandler.AsyncResponse()
}

// SetAsyncResponse sets default AsyncResponse flag.
func SetAsyncResponse(async bool) {
	DefaultHandler.SetAsyncResponse(async)
}

// SetReaderWrapper registers default reader wrapper for net.Conn.
func SetReaderWrapper(wrapper func(conn net.Conn) io.Reader) {
	DefaultHandler.SetReaderWrapper(wrapper)
}

// RecvBufferSize returns default client's read buffer size.
func RecvBufferSize() int {
	return DefaultHandler.RecvBufferSize()
}

// SetRecvBufferSize sets default client's read buffer size.
func SetRecvBufferSize(size int) {
	DefaultHandler.SetRecvBufferSize(size)
}

// SendBufferSize returns default client's read buffer size.
func SendBufferSize() int {
	return DefaultHandler.SendBufferSize()
}

// SetSendBufferSize sets default client's read buffer size.
func SetSendBufferSize(size int) {
	DefaultHandler.SetSendBufferSize(size)
}

// ReadTimeout returns client's read timeout.
func ReadTimeout() time.Duration {
	return DefaultHandler.ReadTimeout()
}

// SetReadTimeout sets client's read timeout.
func SetReadTimeout(timeout time.Duration) {
	DefaultHandler.SetReadTimeout(timeout)
}

// WriteTimeout returns client's write timeout.
func WriteTimeout() time.Duration {
	return DefaultHandler.WriteTimeout()
}

// SetWriteTimeout sets client's write timeout.
func SetWriteTimeout(timeout time.Duration) {
	DefaultHandler.SetWriteTimeout(timeout)
}

// SendQueueSize returns default client's send queue channel capacity.
func SendQueueSize() int {
	return DefaultHandler.SendQueueSize()
}

// SetSendQueueSize sets default client's send queue channel capacity.
func SetSendQueueSize(size int) {
	DefaultHandler.SetSendQueueSize(size)
}

func MaxBodyLen() int {
	return DefaultHandler.MaxBodyLen()
}

func SetMaxBodyLen(l int) {
	DefaultHandler.SetMaxBodyLen(l)
}

// Use registers default method/router handler middleware.
func Use(h HandlerFunc) {
	DefaultHandler.Use(h)
}

// UseCoder registers default message coding middleware,
// coder.Encode will be called before message send,
// coder.Decode will be called after message recv.
func UseCoder(coder MessageCoder) {
	DefaultHandler.UseCoder(coder)
}

// Handle registers default method/router handler.
//
// If pass a Boolean value of "true", the handler will be called asynchronously in a new goroutine,
// Else the handler will be called synchronously in the client's reading goroutine one by one.
func Handle(m string, h HandlerFunc, args ...interface{}) {
	DefaultHandler.Handle(m, h, args...)
}

// HandleNotFound registers default "" method/router handler,
// It will be called when mothod/router is not found.
func HandleNotFound(h HandlerFunc) {
	DefaultHandler.HandleNotFound(h)
}

// HandleMalloc registers default buffer maker.
func HandleMalloc(f func(int) []byte) {
	DefaultHandler.HandleMalloc(f)
}

// EnablePool registers handlers for pool operation for Context and Message and Message.Buffer
func EnablePool(enable bool) {
	DefaultHandler.EnablePool(enable)
}

// SetAsyncExecutor sets executor.
// AsyncExecute executes a func
func SetAsyncExecutor(executor func(f func())) {
	DefaultHandler.SetAsyncExecutor(executor)
}

// AsyncExecute executes a func.
func AsyncExecute(f func()) {
	DefaultHandler.AsyncExecute(f)
}

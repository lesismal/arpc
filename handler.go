// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bufio"
	"fmt"
	"io"
	"net"

	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

// DefaultHandler instance
var DefaultHandler Handler = NewHandler()

// HandlerFunc type define
type HandlerFunc func(*Context)

// RouterHandler handle message
type RouterHandler struct {
	Async    bool
	Handlers []HandlerFunc
}

// Handler defines net message handler
type Handler interface {
	// Clone returns a copy
	Clone() Handler

	// LogTag returns log tag value
	LogTag() string
	// SetLogTag value
	SetLogTag(tag string)

	// HandleConnected registers callback on connected
	HandleConnected(onConnected func(*Client))
	// OnConnected would be called when Client connected
	OnConnected(c *Client)

	// HandleDisconnected registers callback on disconnected
	HandleDisconnected(onDisConnected func(*Client))
	// OnDisconnected would be called when Client disconnected
	OnDisconnected(c *Client)

	// HandleOverstock registers callback on Client chSend overstock
	HandleOverstock(onOverstock func(c *Client, m *Message))
	// OnOverstock would be called when Client chSend is full
	OnOverstock(c *Client, m *Message)

	// HandleMessageDropped registers callback on message dropped
	HandleMessageDropped(onOverstock func(c *Client, m *Message))
	// OnOverstock would be called when message is dropped
	OnMessageDropped(c *Client, m *Message)

	// HandleSessionMiss registers callback on async message seq not found
	HandleSessionMiss(onSessionMiss func(c *Client, m *Message))
	// OnSessionMiss would be called when Client async message seq not found
	OnSessionMiss(c *Client, m *Message)

	// BeforeRecv registers callback before Recv
	BeforeRecv(h func(net.Conn) error)
	// BeforeSend registers callback before Send
	BeforeSend(h func(net.Conn) error)

	// BatchRecv flag
	BatchRecv() bool
	// SetBatchRecv flag
	SetBatchRecv(batch bool)
	// BatchSend flag
	BatchSend() bool
	// SetBatchSend flag
	SetBatchSend(batch bool)

	// AsyncResponse flag
	AsyncResponse() bool
	// SetAsyncResponse flag
	SetAsyncResponse(async bool)

	// WrapReader wraps net.Conn to Read data with io.Reader, buffer e.g.
	WrapReader(conn net.Conn) io.Reader
	// SetReaderWrapper sets reader wrapper
	SetReaderWrapper(wrapper func(conn net.Conn) io.Reader)

	// Recv reads and returns a message from a client
	Recv(c *Client) (*Message, error)
	// Send writes a message to a connection
	Send(c net.Conn, buf []byte) (int, error)
	// SendN writes batch messages to a connection
	SendN(conn net.Conn, buffers net.Buffers) (int, error)

	// RecvBufferSize returns Client.Reader size
	RecvBufferSize() int
	// SetRecvBufferSize sets Client.Reader size
	SetRecvBufferSize(size int)

	// SendQueueSize returns Client.chSend capacity
	SendQueueSize() int
	// SetSendQueueSize sets Client.chSend capacity
	SetSendQueueSize(size int)

	// Use sets middleware
	Use(h HandlerFunc)

	// UseCoder sets middleware for message encoding/decoding
	UseCoder(coder MessageCoder)

	// Coders returns encoding/decoding middlewares
	Coders() []MessageCoder

	// Handle registers method handler
	Handle(m string, h HandlerFunc, args ...interface{})

	// HandleNotFound registers "" method handler
	HandleNotFound(h HandlerFunc)

	// OnMessage dispatches messages
	OnMessage(c *Client, m *Message)

	// GetBuffer factory
	GetBuffer(size int) []byte

	// SetBufferFactory registers buffer factory handler
	SetBufferFactory(f func(int) []byte)
}

type handler struct {
	logtag         string
	batchRecv      bool
	batchSend      bool
	asyncResponse  bool
	recvBufferSize int
	sendQueueSize  int

	onConnected      func(*Client)
	onDisConnected   func(*Client)
	onOverstock      func(c *Client, m *Message)
	onMessageDropped func(c *Client, m *Message)
	onSessionMiss    func(c *Client, m *Message)

	beforeRecv    func(net.Conn) error
	beforeSend    func(net.Conn) error
	bufferFactory func(int) []byte

	wrapReader func(conn net.Conn) io.Reader

	middles   []HandlerFunc
	msgCoders []MessageCoder

	routes map[string]*RouterHandler
}

func (h *handler) Clone() Handler {
	cp := *h
	cp.middles = make([]HandlerFunc, len(h.middles))
	copy(cp.middles, h.middles)

	cp.msgCoders = make([]MessageCoder, len(h.msgCoders))
	copy(cp.msgCoders, h.msgCoders)

	cp.routes = map[string]*RouterHandler{}
	for k, v := range h.routes {
		rh := &RouterHandler{
			Async:    v.Async,
			Handlers: make([]HandlerFunc, len(v.Handlers)),
		}
		copy(rh.Handlers, v.Handlers)
		cp.routes[k] = rh
	}

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

func (h *handler) HandleOverstock(onOverstock func(c *Client, m *Message)) {
	h.onOverstock = onOverstock
}

func (h *handler) OnOverstock(c *Client, m *Message) {
	if h.onOverstock != nil {
		h.onOverstock(c, m)
	}
}

func (h *handler) HandleMessageDropped(onMessageDropped func(c *Client, m *Message)) {
	h.onMessageDropped = onMessageDropped
}

func (h *handler) OnMessageDropped(c *Client, m *Message) {
	if h.onMessageDropped != nil {
		h.onMessageDropped(c, m)
	}
}

func (h *handler) HandleSessionMiss(onSessionMiss func(c *Client, m *Message)) {
	h.onSessionMiss = onSessionMiss
}

func (h *handler) OnSessionMiss(c *Client, m *Message) {
	if h.onSessionMiss != nil {
		h.onSessionMiss(c, m)
	}
}

func (h *handler) BeforeRecv(hb func(net.Conn) error) {
	h.beforeRecv = hb
}

func (h *handler) BeforeSend(hs func(net.Conn) error) {
	h.beforeSend = hs
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

func (h *handler) SendQueueSize() int {
	return h.sendQueueSize
}

func (h *handler) SetSendQueueSize(size int) {
	h.sendQueueSize = size
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
		rh := &RouterHandler{
			Async:    v.Async,
			Handlers: make([]HandlerFunc, len(v.Handlers)+1),
		}
		copy(rh.Handlers, v.Handlers)
		rh.Handlers[len(v.Handlers)] = cbWithNext
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
		h.routes = map[string]*RouterHandler{}
	}
	if len(method) > MaxMethodLen {
		panic(fmt.Errorf("invalid method length %v(> MaxMethodLen %v)", len(method), MaxMethodLen))
	}

	if _, ok := h.routes[""]; !ok {
		rh := &RouterHandler{
			Async:    false,
			Handlers: make([]HandlerFunc, len(h.middles)+1),
		}
		copy(rh.Handlers, h.middles)
		rh.Handlers[len(h.middles)] = func(ctx *Context) {
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
	rh := &RouterHandler{
		Async:    async,
		Handlers: make([]HandlerFunc, len(h.middles)+1),
	}
	copy(rh.Handlers, h.middles)
	rh.Handlers[len(h.middles)] = func(ctx *Context) {
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

	_, err = io.ReadFull(c.Reader, c.Head[:HeaderIndexBodyLenEnd])
	if err != nil {
		return nil, err
	}

	message, err = c.Head.message(h)
	if err != nil {
		return nil, err
	}

	if message.Len() > HeadLen {
		_, err = io.ReadFull(c.Reader, message.Buffer[HeaderIndexBodyLenEnd:])
	}

	return message, err
}

func (h *handler) Send(conn net.Conn, buffer []byte) (int, error) {
	if h.beforeSend != nil {
		if err := h.beforeSend(conn); err != nil {
			return -1, err
		}
	}

	return conn.Write(buffer)
}

func (h *handler) SendN(conn net.Conn, buffers net.Buffers) (int, error) {
	if h.beforeSend != nil {
		if err := h.beforeSend(conn); err != nil {
			return -1, err
		}
	}

	n64, err := buffers.WriteTo(conn)
	return int(n64), err
}

func (h *handler) OnMessage(c *Client, msg *Message) {
	defer util.Recover()

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
			ctx := newContext(c, msg, rh.Handlers)
			if !rh.Async {
				ctx.Next()
			} else {
				go ctx.Next()
			}
		} else {
			if cmd == CmdRequest {
				if rh, ok = h.routes[""]; ok {
					ctx := newContext(c, msg, rh.Handlers)
					ctx.Next()
				} else {
					ctx := newContext(c, msg, rh.Handlers)
					ctx.Error(ErrMethodNotFound)
				}
			}
			log.Warn("%v OnMessage: invalid method: [%v], no handler", h.LogTag(), method)
		}
		break
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
			handler, ok := c.getAndDeleteAsyncHandler(msg.Seq())
			if ok {
				ctx := newContext(c, msg, nil)
				handler(ctx)
			} else {
				h.OnSessionMiss(c, msg)
				log.Warn("%v OnMessage: async handler not exist or expired", h.LogTag())
			}
		}
		break
	default:
		log.Warn("%v OnMessage: invalid cmd [%v]", h.LogTag(), msg.Cmd())
		break
	}
}

func (h *handler) GetBuffer(size int) []byte {
	if h.bufferFactory != nil {
		return h.bufferFactory(size)
	}
	return make([]byte, size)
}

func (h *handler) SetBufferFactory(f func(int) []byte) {
	h.bufferFactory = f
}

// NewHandler factory
func NewHandler() Handler {
	h := &handler{
		logtag:         "[ARPC CLI]",
		batchRecv:      true,
		batchSend:      true,
		asyncResponse:  false,
		recvBufferSize: 8192,
		sendQueueSize:  4096,
	}
	h.wrapReader = func(conn net.Conn) io.Reader {
		return bufio.NewReaderSize(conn, h.recvBufferSize)
	}
	return h
}

// SetHandler sets default handler
func SetHandler(h Handler) {
	DefaultHandler = h
}

// SetLogTag value for DefaultHandler
func SetLogTag(tag string) {
	DefaultHandler.SetLogTag(tag)
}

// HandleConnected registers callback on connected for DefaultHandler
func HandleConnected(onConnected func(*Client)) {
	DefaultHandler.HandleConnected(onConnected)
}

// HandleDisconnected registers callback on disconnected for DefaultHandler
func HandleDisconnected(onDisConnected func(*Client)) {
	DefaultHandler.HandleDisconnected(onDisConnected)
}

// HandleOverstock registers callback on Client chSend overstock for DefaultHandler
func HandleOverstock(onOverstock func(c *Client, m *Message)) {
	DefaultHandler.HandleOverstock(onOverstock)
}

// HandleMessageDropped registers callback on message dropped for DefaultHandler
func HandleMessageDropped(onOverstock func(c *Client, m *Message)) {
	DefaultHandler.HandleMessageDropped(onOverstock)
}

// HandleSessionMiss registers callback on async message seq not found for DefaultHandler
func HandleSessionMiss(onSessionMiss func(c *Client, m *Message)) {
	DefaultHandler.HandleSessionMiss(onSessionMiss)
}

// BeforeRecv registers callback before Recv for DefaultHandler
func BeforeRecv(h func(net.Conn) error) {
	DefaultHandler.BeforeRecv(h)
}

// BeforeSend registers callback before Send for DefaultHandler
func BeforeSend(h func(net.Conn) error) {
	DefaultHandler.BeforeSend(h)
}

// BatchRecv flag
func BatchRecv() bool {
	return DefaultHandler.BatchRecv()
}

// SetBatchRecv flag for DefaultHandler
func SetBatchRecv(batch bool) {
	DefaultHandler.SetBatchRecv(batch)
}

// BatchSend flag
func BatchSend() bool {
	return DefaultHandler.BatchSend()
}

// SetBatchSend flag for DefaultHandler
func SetBatchSend(batch bool) {
	DefaultHandler.SetBatchSend(batch)
}

// AsyncResponse flag
func AsyncResponse() bool {
	return DefaultHandler.AsyncResponse()
}

// SetAsyncResponse flag for DefaultHandler
func SetAsyncResponse(async bool) {
	DefaultHandler.SetAsyncResponse(async)
}

// SetReaderWrapper sets reader wrapper for DefaultHandler
func SetReaderWrapper(wrapper func(conn net.Conn) io.Reader) {
	DefaultHandler.SetReaderWrapper(wrapper)
}

// RecvBufferSize returns Client.Reader size
func RecvBufferSize() int {
	return DefaultHandler.RecvBufferSize()
}

// SetRecvBufferSize sets Client.Reader size for DefaultHandler
func SetRecvBufferSize(size int) {
	DefaultHandler.SetRecvBufferSize(size)
}

// SendQueueSize returns Client.chSend capacity
func SendQueueSize() int {
	return DefaultHandler.SendQueueSize()
}

// SetSendQueueSize sets Client.chSend capacity for DefaultHandler
func SetSendQueueSize(size int) {
	DefaultHandler.SetSendQueueSize(size)
}

// Use sets middleware for DefaultHandler
func Use(h HandlerFunc) {
	DefaultHandler.Use(h)
}

// UseCoder sets middleware for message encoding/decoding for DefaultHandler
func UseCoder(coder MessageCoder) {
	DefaultHandler.UseCoder(coder)
}

// Handle registers method handler for DefaultHandler
func Handle(m string, h HandlerFunc, args ...interface{}) {
	DefaultHandler.Handle(m, h, args...)
}

// HandleNotFound registers "" method handler for DefaultHandler
func HandleNotFound(h HandlerFunc) {
	DefaultHandler.HandleNotFound(h)
}

// SetBufferFactory registers buffer factory handler for DefaultHandler
func SetBufferFactory(f func(int) []byte) {
	DefaultHandler.SetBufferFactory(f)
}

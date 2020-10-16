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

	// HandleOverstock registers callback on Client chSend overstockll
	HandleOverstock(onOverstock func(c *Client, m *Message))
	// OnOverstock would be called when Client chSend is full
	OnOverstock(c *Client, m *Message)

	// HandleSessionMiss registers callback on async message seq not found
	HandleSessionMiss(onSessionMiss func(c *Client, m *Message))
	// OnSessionMiss would be called when Client async message seq not found
	OnSessionMiss(c *Client, m *Message)

	// BeforeRecv registers callback before Recv
	BeforeRecv(bh func(net.Conn) error)
	// BeforeSend registers callback before Send
	BeforeSend(bh func(net.Conn) error)

	// BatchRecv flag
	BatchRecv() bool
	// SetBatchRecv flag
	SetBatchRecv(batch bool)
	// BatchSend flag
	BatchSend() bool
	// SetBatchSend flag
	SetBatchSend(batch bool)

	// WrapReader wraps net.Conn to Read data with io.Reader, buffer e.g.
	WrapReader(conn net.Conn) io.Reader
	// SetReaderWrapper sets reader wrapper
	SetReaderWrapper(wrapper func(conn net.Conn) io.Reader)

	// Recv reads and returns a message from a client
	Recv(c *Client) (*Message, error)
	// Send writes a message to a connection
	Send(c net.Conn, m *Message) (int, error)
	// SendN writes batch messages to a connection
	SendN(conn net.Conn, messages []*Message, buffers net.Buffers) (int, error)

	// RecvBufferSize returns Client.Reader size
	RecvBufferSize() int
	// SetRecvBufferSize sets Client.Reader size
	SetRecvBufferSize(size int)

	// SendQueueSize returns Client.chSend capacity
	SendQueueSize() int
	// SetSendQueueSize sets Client.chSend capacity
	SetSendQueueSize(size int)

	// Handle sets middleware
	Use(h HandlerFunc)

	// UseCoder sets middleware for message encoding/decoding
	UseCoder(coder MessageCoder)

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
	recvBufferSize int
	sendQueueSize  int

	onConnected    func(*Client)
	onDisConnected func(*Client)
	onOverstock    func(c *Client, m *Message)
	onSessionMiss  func(c *Client, m *Message)

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
	h.onConnected = onConnected
}

func (h *handler) OnConnected(c *Client) {
	if h.onConnected != nil {
		h.onConnected(c)
	}
}

func (h *handler) HandleDisconnected(onDisConnected func(*Client)) {
	h.onDisConnected = onDisConnected
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

func (h *handler) HandleSessionMiss(onSessionMiss func(c *Client, m *Message)) {
	h.onSessionMiss = onSessionMiss
}

func (h *handler) OnSessionMiss(c *Client, m *Message) {
	if h.onSessionMiss != nil {
		h.onSessionMiss(c, m)
	}
}

func (h *handler) BeforeRecv(bh func(net.Conn) error) {
	h.beforeRecv = bh
}

func (h *handler) BeforeSend(bh func(net.Conn) error) {
	h.beforeSend = bh
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
	h.msgCoders = append(h.msgCoders, coder)
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

	async := false
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

	for i := len(h.msgCoders) - 1; i >= 0; i-- {
		message = h.msgCoders[i].Decode(message)
	}

	return message, err
}

func (h *handler) Send(conn net.Conn, msg *Message) (int, error) {
	if h.beforeSend != nil {
		if err := h.beforeSend(conn); err != nil {
			return -1, err
		}
	}

	for i := 0; i < len(h.msgCoders); i++ {
		msg = h.msgCoders[i].Encode(msg)
	}

	return conn.Write(msg.Buffer)
}

func (h *handler) SendN(conn net.Conn, messages []*Message, buffers net.Buffers) (int, error) {
	if h.beforeSend != nil {
		if err := h.beforeSend(conn); err != nil {
			return -1, err
		}
	}

	for i := 0; i < len(messages); i++ {
		for j := 0; j < len(h.msgCoders); j++ {
			messages[i] = h.msgCoders[j].Encode(messages[i])
			buffers = append(buffers, messages[i].Buffer)
		}
	}

	n64, err := buffers.WriteTo(conn)
	return int(n64), err
}

func (h *handler) OnMessage(c *Client, msg *Message) {
	ml := msg.MethodLen()
	if ml <= 0 || ml > MaxMethodLen || ml > (msg.Len()-HeadLen) {
		log.Warn("%v OnMessage: invalid request method length %v, dropped", h.LogTag(), ml)
		return
	}
	// log.Info("---- ml: %v", ml)
	cmd := msg.Cmd()
	switch cmd {
	case CmdRequest, CmdNotify:
		method := msg.method()
		if rh, ok := h.routes[method]; ok {
			ctx := newContext(c, msg, rh.Handlers)
			if !rh.Async {
				ctx.Next()
				if cmd == CmdRequest {
					ctx.Dump()
				}
			} else {
				go func() {
					ctx.Next()
					if cmd == CmdRequest {
						ctx.Dump()
					}
				}()
			}
		} else {
			if rh, ok = h.routes[""]; ok {
				ctx := newContext(c, msg, rh.Handlers)
				ctx.Next()
				if cmd == CmdRequest {
					ctx.Dump()
				}
			} else {
				ctx := newContext(c, msg, nil)
				ctx.Error(ErrMethodNotFound)
				if cmd == CmdRequest {
					ctx.Dump()
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
				handler(newContext(c, msg, nil))
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
		recvBufferSize: 4096,
		sendQueueSize:  1024,
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

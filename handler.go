// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bufio"
	"fmt"
	"io"
	"net"
)

// DefaultHandler instance
var DefaultHandler Handler = NewHandler()

// HandlerFunc type define
type HandlerFunc func(*Context)

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
	HandleOverstock(onOverstock func(c *Client, m Message))
	// OnOverstock would be called when Client chSend is full
	OnOverstock(c *Client, m Message)

	// HandleSessionMiss registers callback on async message seq not found
	HandleSessionMiss(onSessionMiss func(c *Client, m Message))
	// OnSessionMiss would be called when Client async message seq not found
	OnSessionMiss(c *Client, m Message)

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
	Recv(c *Client) (Message, error)
	// Send writes a message to a connection
	Send(c net.Conn, m Message) (int, error)
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

	// Handle registers method handler
	Handle(m string, h HandlerFunc)

	// OnMessage dispatches messages
	OnMessage(c *Client, m Message)
}

type handler struct {
	logtag         string
	batchRecv      bool
	batchSend      bool
	recvBufferSize int
	sendQueueSize  int

	onConnected    func(*Client)
	onDisConnected func(*Client)
	onOverstock    func(c *Client, m Message)
	onSessionMiss  func(c *Client, m Message)

	beforeRecv func(net.Conn) error
	beforeSend func(net.Conn) error

	wrapReader func(conn net.Conn) io.Reader

	routes map[string]HandlerFunc
}

func (h *handler) Clone() Handler {
	var cp = *h
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

func (h *handler) HandleOverstock(onOverstock func(c *Client, m Message)) {
	h.onOverstock = onOverstock
}

func (h *handler) OnOverstock(c *Client, m Message) {
	if h.onOverstock != nil {
		h.onOverstock(c, m)
	}
}

func (h *handler) HandleSessionMiss(onSessionMiss func(c *Client, m Message)) {
	h.onSessionMiss = onSessionMiss
}

func (h *handler) OnSessionMiss(c *Client, m Message) {
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

func (h *handler) Handle(method string, cb HandlerFunc) {
	if h.routes == nil {
		h.routes = map[string]HandlerFunc{}
	}
	if len(method) > MaxMethodLen {
		panic(fmt.Errorf("invalid method length %v(> MaxMethodLen %v)", len(method), MaxMethodLen))
	}
	if _, ok := h.routes[method]; ok {
		panic(fmt.Errorf("handler exist for method %v ", method))
	}
	h.routes[method] = cb
}

func (h *handler) Recv(c *Client) (Message, error) {
	var (
		err     error
		message Message
	)

	if h.beforeRecv != nil {
		if err = h.beforeRecv(c.Conn); err != nil {
			return nil, err
		}
	}

	_, err = io.ReadFull(c.Reader, c.Head)
	if err != nil {
		return nil, err
	}

	message, err = c.Head.message()
	if err == nil && len(message) > HeadLen {
		_, err = io.ReadFull(c.Reader, message[HeadLen:])
	}

	return message, err
}

func (h *handler) Send(conn net.Conn, m Message) (int, error) {
	if h.beforeSend != nil {
		if err := h.beforeSend(conn); err != nil {
			return -1, err
		}
	}
	return conn.Write(m)
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

func (h *handler) OnMessage(c *Client, msg Message) {
	// cmd, seq, isAsync, method, body, err := msg.parse()
	switch msg.Cmd() {
	case CmdRequest, CmdNotify:
		if msg.MethodLen() == 0 {
			logWarn("%v OnMessage: invalid request message with 0 method length, dropped", h.LogTag())
			return
		}
		method := msg.Method()
		if handler, ok := h.routes[method]; ok {
			ctx := ctxGet(c, msg)
			defer func() {
				ctxPut(ctx)
				memPut(msg)
			}()
			defer handlePanic()
			handler(ctx)
		} else {
			memPut(msg)
			logWarn("%v OnMessage: invalid method: [%v], no handler", h.LogTag(), method)
		}
	case CmdResponse:
		if msg.MethodLen() > 0 {
			logWarn("%v OnMessage: invalid response message with method length %v, dropped", h.LogTag(), msg.MethodLen())
			return
		}
		if !msg.IsAsync() {
			seq := msg.Seq()
			session, ok := c.getSession(seq)
			if ok {
				session.done <- msg
			} else {
				memPut(msg)
				logWarn("%v OnMessage: session not exist or expired", h.LogTag())
			}
		} else {
			handler, ok := c.getAndDeleteAsyncHandler(msg.Seq())
			if ok {
				ctx := ctxGet(c, msg)
				defer func() {
					ctxPut(ctx)
					memPut(msg)
				}()
				defer handlePanic()
				handler(ctx)
			} else {
				memPut(msg)
				logWarn("%v OnMessage: async handler not exist or expired", h.LogTag())
			}
		}
	default:
		memPut(msg)
		logWarn("%v OnMessage: invalid cmd [%v]", h.LogTag(), msg.Cmd())
	}
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

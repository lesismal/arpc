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

// Handler defines net message handler
type Handler interface {
	// BeforeRecv registers callback before Recv
	BeforeRecv(bh func(net.Conn) error)

	// BeforeSend registers callback before Send
	BeforeSend(bh func(net.Conn) error)

	// WrapReader wraps net.Conn to Read data with io.Reader, buffer e.g.
	WrapReader(conn net.Conn) io.Reader

	// Recv reads and returns a message from a client
	Recv(c *Client) (Message, error)

	// Send writes a message to a connection
	Send(c net.Conn, m Message) (int, error)

	// SendQueueSize returns Client's chSend capacity
	SendQueueSize() int
	// SetSendQueueSize sets Client's chSend capacity
	SetSendQueueSize(size int)

	// Handle registers method handler
	Handle(m string, h func(*Context))

	// OnMessage dispatches messages
	OnMessage(c *Client, m Message)
}

type handler struct {
	beforeRecv    func(net.Conn) error
	beforeSend    func(net.Conn) error
	routes        map[string]func(*Context)
	sendQueueSize int
}

func (h *handler) BeforeRecv(bh func(net.Conn) error) {
	h.beforeRecv = bh
}

func (h *handler) BeforeSend(bh func(net.Conn) error) {
	h.beforeSend = bh
}

func (h *handler) WrapReader(conn net.Conn) io.Reader {
	return bufio.NewReaderSize(conn, 1024)
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

func (h *handler) SendQueueSize() int {
	return h.sendQueueSize
}

func (h *handler) SetSendQueueSize(size int) {
	h.sendQueueSize = size
}

func (h *handler) Handle(method string, cb func(*Context)) {
	if h.routes == nil {
		h.routes = map[string]func(*Context){}
	}
	if len(method) > MaxMethodLen {
		panic(fmt.Errorf("invalid method length %v(> MaxMethodLen %v)", len(method), MaxMethodLen))
	}
	if _, ok := h.routes[method]; ok {
		panic(fmt.Errorf("handler exist for method %v ", method))
	}
	h.routes[method] = cb
}

func (h *handler) OnMessage(c *Client, msg Message) {
	cmd, seq, isAsync, method, body, err := msg.Parse()
	switch cmd {
	case RPCCmdReq:
		defer memPut(msg)
		if cb, ok := h.routes[method]; ok {
			defer handlePanic()
			cb(NewContext(c, msg))
		} else {
			DefaultLogger.Info("invalid method: [%v], %v, %v", method, body, err)
		}
	case RPCCmdRsp, RPCCmdErr:
		if !isAsync {
			session, ok := c.getSession(seq)
			if ok {
				session.done <- msg
			} else {
				DefaultLogger.Info("session not exist or expired: [seq: %v] [len(body): %v] [%v]", seq, len(body), err)
			}
		} else {
			handler, ok := c.getAndDeleteAsyncHandler(seq)
			if ok {
				defer memPut(msg)
				defer handlePanic()
				handler(&Context{Client: c, Message: msg})
			} else {
				DefaultLogger.Info("asyncHandler not exist or expired: [seq: %v] [len(body): %v, %v] [%v]", seq, len(body), string(body), err)
			}
		}
	default:
		defer memPut(msg)
		DefaultLogger.Info("invalid cmd: [%v]", cmd)
	}
}

// NewHandler factory
func NewHandler() Handler {
	return &handler{
		sendQueueSize: 1024,
	}
}

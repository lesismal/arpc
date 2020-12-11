// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package websocket

import (
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var (
	// ErrClosed .
	ErrClosed = errors.New("websocket listener closed")
	// ErrInvalidMessage .
	ErrInvalidMessage = errors.New("invalid message")
	// ErrInvalidMessageType .
	ErrInvalidMessageType = errors.New("invalid message type")
)

// Listener .
type Listener struct {
	addr        net.Addr
	upgrader    *websocket.Upgrader
	acceptQueue chan net.Conn
}

// Handler .
func (ln *Listener) Handler(w http.ResponseWriter, r *http.Request) {
	c, err := ln.upgrader.Upgrade(w, r, nil)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	defer c.Close()

	wsc := &Conn{Conn: c, chHandler: make(chan func(), 1)}
	ln.acceptQueue <- wsc
	timeout := time.NewTimer(time.Second)
	select {
	case handler := <-wsc.chHandler:
		timeout.Stop()
		handler()
	case <-timeout.C:
	}
}

// Close .
func (ln *Listener) Close() error {
	close(ln.acceptQueue)
	return nil
}

// Addr .
func (ln *Listener) Addr() net.Addr {
	return ln.addr
}

// Accept .
func (ln *Listener) Accept() (net.Conn, error) {
	c := <-ln.acceptQueue
	if c != nil {
		return c, nil
	}
	return nil, ErrClosed
}

// Conn wraps websocket.Conn to net.Conn
type Conn struct {
	*websocket.Conn
	chHandler chan func()
	buffer    []byte
}

// HandleWebsocket .
func (c *Conn) HandleWebsocket(handler func()) {
	select {
	case c.chHandler <- handler:
	default:
	}
}

// Read .
func (c *Conn) Read(b []byte) (int, error) {
	var (
		err error
	)
	if len(c.buffer) == 0 {
		_, c.buffer, err = c.ReadMessage()
		if err != nil {
			return 0, err
		}
	}

	cbl := len(c.buffer)
	if cbl <= len(b) {
		copy(b[:cbl], c.buffer)
		c.buffer = nil
		return cbl, nil
	}
	copy(b, c.buffer[:len(b)])
	c.buffer = c.buffer[len(b):]
	return len(b), nil
}

// Write .
func (c *Conn) Write(b []byte) (int, error) {
	err := c.WriteMessage(websocket.BinaryMessage, b)
	if err == nil {
		return len(b), nil
	}
	return 0, err
}

// SetDeadline .
func (c *Conn) SetDeadline(t time.Time) error {
	err := c.SetReadDeadline(t)
	if err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// Listen wraps websocket listen
func Listen(addr string, upgrader *websocket.Upgrader) (net.Listener, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	if upgrader == nil {
		upgrader = &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}
	}
	ln := &Listener{
		addr:        tcpAddr,
		upgrader:    upgrader,
		acceptQueue: make(chan net.Conn, 4096),
	}
	return ln, nil
}

// Dial wraps websocket dial
func Dial(url string) (net.Conn, error) {
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	return &Conn{Conn: c}, nil
}

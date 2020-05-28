// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// TimeZero definition
	TimeZero time.Duration = 0
	// TimeForever definition
	TimeForever time.Duration = 1<<63 - 1
)

type rpcSession struct {
	seq  uint64
	done chan Message
}

func newSession(seq uint64) *rpcSession {
	return &rpcSession{seq: seq, done: make(chan Message, 1)}
}

// Client defines rpc client struct
type Client struct {
	Conn     net.Conn
	Reader   io.Reader
	head     [HeadLen]byte
	Head     Header
	Codec    Codec
	Handler  Handler
	Dialer   func() (net.Conn, error)
	UserData interface{}

	running      bool
	reconnecting bool

	mux             sync.RWMutex
	seq             uint64
	sessionMap      map[uint64]*rpcSession
	asyncHandlerMap map[uint64]HandlerFunc

	chSend chan Message

	onStop func() int64
}

// Run client
func (c *Client) Run() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		go c.sendLoop()
		go c.recvLoop()
	}
}

// Stop client
func (c *Client) Stop() {
	c.mux.Lock()
	defer c.mux.Unlock()

	if c.running {
		c.running = false
		c.Conn.Close()
		if c.chSend != nil {
			close(c.chSend)
		}
		if c.onStop != nil {
			c.onStop()
		}
		c.Handler.OnDisconnected(c)
	}
}

// Call make rpc call
func (c *Client) Call(method string, req interface{}, rsp interface{}, timeout time.Duration) error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}
	if timeout <= 0 {
		return fmt.Errorf("invalid timeout arg: %v", timeout)
	}

	timer := time.NewTimer(timeout)

	msg := c.newReqMessage(CmdRequest, method, req, 0)

	seq := msg.Seq()
	sess := newSession(seq)
	c.addSession(seq, sess)
	defer func() {
		timer.Stop()
		c.mux.Lock()
		delete(c.sessionMap, seq)
		c.mux.Unlock()
	}()

	select {
	case c.chSend <- msg:
	case <-timer.C:
		c.Handler.OnOverstock(c, msg)
		return ErrClientTimeout
	}

	select {
	// response msg
	case msg = <-sess.done:
	case <-timer.C:
		return ErrClientTimeout
	}

	switch msg.Cmd() {
	case CmdResponse:
		if msg.IsError() {
			return msg.Error()
		}
		if rsp != nil {
			switch vt := rsp.(type) {
			case *string:
				*vt = string(msg[HeadLen:])
			case *[]byte:
				*vt = msg[HeadLen:]
			// case *error:
			// 	*vt = msg.Error()
			default:
				return c.Codec.Unmarshal(msg[HeadLen:], rsp)
			}
		}
	default:
		return ErrInvalidRspMessage
	}

	return nil
}

// CallAsync make async rpc call
func (c *Client) CallAsync(method string, req interface{}, handler HandlerFunc, timeout time.Duration) error {
	return c.callAsync(CmdRequest, method, req, handler, timeout)
}

// Notify make rpc notify
func (c *Client) Notify(method string, data interface{}, timeout time.Duration) error {
	return c.callAsync(CmdNotify, method, data, nil, timeout)
}

// PushMsg push msg to client's send queue
func (c *Client) PushMsg(msg Message, timeout time.Duration) error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}
	if timeout < 0 {
		return fmt.Errorf("invalid timeout arg: %v", timeout)
	}

	switch timeout {
	case TimeZero:
		select {
		case c.chSend <- msg:
		default:
			c.Handler.OnOverstock(c, msg)
			return ErrClientOverstock
		}
	default:
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case c.chSend <- msg:
		case <-timer.C:
			c.Handler.OnOverstock(c, msg)
			return ErrClientTimeout
		}
	}

	return nil
}

func (c *Client) callAsync(cmd byte, method string, req interface{}, handler HandlerFunc, timeout time.Duration) error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}
	if timeout < 0 {
		return fmt.Errorf("invalid timeout arg: %v", timeout)
	}

	var (
		msg = c.newReqMessage(cmd, method, req, 1)
		seq = msg.Seq()
	)

	if handler != nil {
		c.addAsyncHandler(seq, handler)
	}

	switch timeout {
	case TimeZero:
		select {
		case c.chSend <- msg:
		default:
			c.Handler.OnOverstock(c, msg)
			if handler != nil {
				c.deleteAsyncHandler(seq)
			}
			return ErrClientOverstock
		}
	default:
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case c.chSend <- msg:
		case <-timer.C:
			c.Handler.OnOverstock(c, msg)
			if handler != nil {
				c.deleteAsyncHandler(seq)
			}
			return ErrClientTimeout
		}
	}

	return nil
}

func (c *Client) addSession(seq uint64, session *rpcSession) {
	c.mux.Lock()
	c.sessionMap[seq] = session
	c.mux.Unlock()
}

func (c *Client) getSession(seq uint64) (*rpcSession, bool) {
	c.mux.Lock()
	session, ok := c.sessionMap[seq]
	c.mux.Unlock()
	return session, ok
}

func (c *Client) deleteSession(seq uint64) {
	c.mux.Lock()
	delete(c.sessionMap, seq)
	c.mux.Unlock()
}

func (c *Client) addAsyncHandler(seq uint64, h HandlerFunc) {
	c.mux.Lock()
	c.asyncHandlerMap[seq] = h
	c.mux.Unlock()
}

func (c *Client) deleteAsyncHandler(seq uint64) {
	c.mux.Lock()
	delete(c.asyncHandlerMap, seq)
	c.mux.Unlock()
}

func (c *Client) getAndDeleteAsyncHandler(seq uint64) (HandlerFunc, bool) {
	c.mux.Lock()
	handler, ok := c.asyncHandlerMap[seq]
	if ok {
		delete(c.asyncHandlerMap, seq)
		c.mux.Unlock()
	} else {
		c.mux.Unlock()
	}

	return handler, ok
}

func (c *Client) clearAsyncHandler() {
	c.mux.Lock()
	c.asyncHandlerMap = make(map[uint64]HandlerFunc)
	c.mux.Unlock()
}

func (c *Client) initReader() {
	if c.Handler.BatchRecv() {
		c.Reader = c.Handler.WrapReader(c.Conn)
	} else {
		c.Reader = c.Conn
	}
}

func (c *Client) recvLoop() {
	var (
		err  error
		msg  Message
		addr = c.Conn.RemoteAddr().String()
	)

	logDebug("%v\t%v\trecvLoop start", c.Handler.LogTag(), addr)
	defer logDebug("%v\t%v\trecvLoop stop", c.Handler.LogTag(), addr)

	if c.Dialer == nil {
		for c.running {
			msg, err = c.Handler.Recv(c)
			if err != nil {
				logInfo("%v\t%v\tDisconnected: %v", c.Handler.LogTag(), addr, err)
				c.Stop()
				return
			}
			c.Handler.OnMessage(c, msg)
		}
	} else {
		go c.Handler.OnConnected(c)

		for c.running {
			for {
				msg, err = c.Handler.Recv(c)
				if err != nil {
					logInfo("%v\t%v\tDisconnected: %v", c.Handler.LogTag(), addr, err)
					break
				}
				c.Handler.OnMessage(c, msg)
			}

			c.reconnecting = true
			c.Conn.Close()

			for c.running {
				logInfo("%v\t%v\tReconnecting ...", c.Handler.LogTag(), addr)
				conn, err := c.Dialer()
				if err == nil {
					c.Conn = conn

					c.initReader()

					c.clearAsyncHandler()

					c.reconnecting = false

					logInfo("%v\t%v\tReconnected", c.Handler.LogTag(), addr)

					go c.Handler.OnConnected(c)

					break
				}

				time.Sleep(time.Second)
			}
		}
	}

}

func (c *Client) sendLoop() {
	addr := c.Conn.RemoteAddr().String()
	logDebug("%v\t%v\tsendLoop start", c.Handler.LogTag(), addr)
	defer logDebug("%v\t%v\tsendLoop stop", c.Handler.LogTag(), addr)

	if !c.Handler.BatchSend() {
		var conn net.Conn
		for msg := range c.chSend {
			conn = c.Conn
			if !c.reconnecting {
				if _, err := c.Handler.Send(conn, msg); err != nil {
					conn.Close()
				}
			}
		}
	} else {
		var conn net.Conn
		var buffers net.Buffers = make([][]byte, 10)[0:0]
		for msg := range c.chSend {
			buffers = append(buffers, msg)
			for i := 1; i < 10; i++ {
				select {
				case msg = <-c.chSend:
					buffers = append(buffers, msg)
				default:
					goto SEND
				}
			}
		SEND:
			conn = c.Conn
			if !c.reconnecting {
				if len(buffers) == 1 {
					if _, err := c.Handler.Send(conn, buffers[0]); err != nil {
						conn.Close()
					}
				} else {
					if _, err := c.Handler.SendN(conn, buffers); err != nil {
						conn.Close()
					}
				}
			}
			buffers = buffers[0:0]
		}
	}
}

func (c *Client) newReqMessage(cmd byte, method string, req interface{}, async byte) Message {
	var (
		data    []byte
		msg     Message
		bodyLen int
	)

	data = valueToBytes(c.Codec, req)

	bodyLen = len(method) + len(data)

	msg = Message(memGet(HeadLen + bodyLen))
	binary.LittleEndian.PutUint32(msg[headerIndexBodyLenBegin:headerIndexBodyLenEnd], uint32(bodyLen))
	binary.LittleEndian.PutUint64(msg[headerIndexSeqBegin:headerIndexSeqEnd], atomic.AddUint64(&c.seq, 1))

	msg[headerIndexCmd] = cmd
	msg[headerIndexAsync] = async
	msg[headerIndexError] = 0
	msg[headerIndexMethodLen] = byte(len(method))
	copy(msg[HeadLen:HeadLen+len(method)], method)
	copy(msg[HeadLen+len(method):], data)

	return msg
}

// newClientWithConn factory
func newClientWithConn(conn net.Conn, codec Codec, handler Handler, onStop func() int64) *Client {
	logInfo("%v\t%v\tConnected", handler.LogTag(), conn.RemoteAddr())

	c := &Client{}
	c.Conn = conn
	c.Head = Header(c.head[:])
	c.Codec = codec
	c.Handler = handler
	c.chSend = make(chan Message, c.Handler.SendQueueSize())
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]HandlerFunc)
	c.onStop = onStop

	return c
}

// NewClient factory
func NewClient(dialer func() (net.Conn, error)) (*Client, error) {
	conn, err := dialer()
	if err != nil {
		return nil, err
	}

	c := &Client{}
	c.Conn = conn

	c.Head = Header(c.head[:])
	c.Codec = DefaultCodec
	c.Handler = DefaultHandler
	c.Dialer = dialer
	c.chSend = make(chan Message, c.Handler.SendQueueSize())
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]HandlerFunc)

	logInfo("%v\t%v\tConnected", c.Handler.LogTag(), conn.RemoteAddr())

	return c, nil
}

// ClientPool definition
type ClientPool struct {
	size    uint64
	round   uint64
	clients []*Client
}

// Size returns a client number
func (pool *ClientPool) Size() int {
	return len(pool.clients)
}

// Get returns a Client instance
func (pool *ClientPool) Get(i int) *Client {
	return pool.clients[uint64(i)%pool.size]
}

// Next returns a Client by round robin
func (pool *ClientPool) Next() *Client {
	i := atomic.AddUint64(&pool.round, 1)
	return pool.clients[i%pool.size]
}

// Handler returns Handler
func (pool *ClientPool) Handler() Handler {
	return pool.Next().Handler
}

// Run all clients
func (pool *ClientPool) Run() {
	for _, c := range pool.clients {
		c.Run()
	}
}

// Stop all clients
func (pool *ClientPool) Stop() {
	for _, c := range pool.clients {
		c.Stop()
	}
}

// NewClientPool factory
func NewClientPool(dialer func() (net.Conn, error), size int) (*ClientPool, error) {
	pool := &ClientPool{
		size:    uint64(size),
		round:   0xFFFFFFFFFFFFFFFF,
		clients: make([]*Client, size),
	}

	for i := 0; i < size; i++ {
		c, err := NewClient(dialer)
		if err != nil {
			for j := 0; j < i; j++ {
				pool.clients[j].Stop()
			}
			return nil, err
		}
		pool.clients[i] = c
	}

	return pool, nil
}

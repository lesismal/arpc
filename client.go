// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

const (
	// TimeZero definition
	TimeZero time.Duration = 0
	// TimeForever definition
	TimeForever time.Duration = 1<<63 - 1
)

// DialerFunc .
type DialerFunc func() (net.Conn, error)

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
	head     [4]byte
	Head     Header
	Codec    codec.Codec
	Handler  Handler
	Dialer   DialerFunc
	UserData interface{}

	running      bool
	reconnecting bool

	mux             sync.RWMutex
	seq             uint64
	sessionMap      map[uint64]*rpcSession
	asyncHandlerMap map[uint64]HandlerFunc

	chSend  chan Message
	chClose chan util.Empty

	onStop func() int64
}

// Run client
func (c *Client) Run() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		go util.Safe(c.sendLoop)
		go util.Safe(c.recvLoop)
	}
}

// RunWebsocket client
func (c *Client) RunWebsocket() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		go util.Safe(c.sendLoop)
		c.Conn.(WebsocketConn).HandleWebsocket(c.recvLoop)
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
			close(c.chClose)
		}
		if c.onStop != nil {
			c.onStop()
		}
		c.Handler.OnDisconnected(c)
	}
}

// Call make rpc call with timeout
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

	ml := len(method)
	if ml <= 0 || ml > MaxMethodLen {
		return fmt.Errorf("invalid method length: %v", ml)
	}

	timer := time.NewTimer(timeout)

	msg := c.newReqMessage(CmdRequest, method, req, false)

	seq := msg.Seq()
	sess := newSession(seq)
	c.addSession(seq, sess)
	defer func() {
		timer.Stop()
		c.deleteSession(seq)
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

	return c.parseResponse(msg, rsp)
}

// CallWith make rpc call with context
func (c *Client) CallWith(ctx context.Context, method string, req interface{}, rsp interface{}) error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}

	ml := len(method)
	if ml <= 0 || ml > MaxMethodLen {
		return fmt.Errorf("invalid method length: %v", ml)
	}

	msg := c.newReqMessage(CmdRequest, method, req, false)

	seq := msg.Seq()
	sess := newSession(seq)
	c.addSession(seq, sess)
	defer c.deleteSession(seq)

	select {
	case c.chSend <- msg:
	case <-ctx.Done():
		c.Handler.OnOverstock(c, msg)
		return ErrClientTimeout
	}

	select {
	// response msg
	case msg = <-sess.done:
	case <-ctx.Done():
		return ErrClientTimeout
	}

	return c.parseResponse(msg, rsp)
}

// CallAsync make async rpc call with timeout
func (c *Client) CallAsync(method string, req interface{}, handler HandlerFunc, timeout time.Duration) error {
	return c.callAsync(CmdRequest, method, req, handler, timeout)
}

// deprecated: can not graceful clear missing asynchandler in time
// CallAsyncWith make async rpc call with context
// func (c *Client) CallAsyncWith(ctx context.Context, method string, req interface{}, handler HandlerFunc) error {
// 	return c.callAsyncWith(ctx, CmdRequest, method, req, handler)
// }

// Notify make rpc notify with timeout
func (c *Client) Notify(method string, data interface{}, timeout time.Duration) error {
	return c.callAsync(CmdNotify, method, data, nil, timeout)
}

// NotifyWith make rpc notify with context
func (c *Client) NotifyWith(ctx context.Context, method string, data interface{}) error {
	return c.callAsyncWith(ctx, CmdNotify, method, data)
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

// NewMessage factory
func (c *Client) NewMessage(cmd byte, method string, v interface{}) Message {
	msg := newMessage(cmd, method, v, c.Handler, c.Codec)
	binary.LittleEndian.PutUint64(msg[HeaderIndexSeqBegin:HeaderIndexSeqEnd], atomic.AddUint64(&c.seq, 1))
	return msg
}

func (c *Client) parseResponse(msg Message, rsp interface{}) error {
	if msg == nil {
		return ErrClientReconnecting
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
				return c.Codec.Unmarshal(msg.Data(), rsp)
			}
		}
	default:
		return ErrInvalidRspMessage
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

	ml := len(method)
	if ml <= 0 || ml > MaxMethodLen {
		return fmt.Errorf("invalid method length: %v", ml)
	}

	msg := c.newReqMessage(cmd, method, req, true)
	seq := msg.Seq()

	if handler != nil {
		c.addAsyncHandler(seq, handler)
		time.AfterFunc(timeout, func() { c.deleteAsyncHandler(seq) })
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

func (c *Client) callAsyncWith(ctx context.Context, cmd byte, method string, req interface{}) error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}

	ml := len(method)
	if ml <= 0 || ml > MaxMethodLen {
		return fmt.Errorf("invalid method length: %v", ml)
	}

	msg := c.newReqMessage(cmd, method, req, true)
	// seq := msg.Seq()

	// if handler != nil {
	// 	c.addAsyncHandler(seq, handler)
	// 	// time.AfterFunc(timeout, func() { c.deleteAsyncHandler(seq) })
	// }

	select {
	case c.chSend <- msg:
	case <-ctx.Done():
		c.Handler.OnOverstock(c, msg)
		// if handler != nil {
		// 	c.deleteAsyncHandler(seq)
		// }
		return ErrClientTimeout
	}

	return nil
}

func (c *Client) addSession(seq uint64, session *rpcSession) {
	c.mux.Lock()
	c.sessionMap[seq] = session
	c.mux.Unlock()
}

func (c *Client) deleteSession(seq uint64) *rpcSession {
	c.mux.Lock()
	session := c.sessionMap[seq]
	delete(c.sessionMap, seq)
	c.mux.Unlock()
	return session
}

func (c *Client) getSession(seq uint64) (*rpcSession, bool) {
	c.mux.Lock()
	session, ok := c.sessionMap[seq]
	c.mux.Unlock()
	return session, ok
}

func (c *Client) clearSession() {
	c.mux.Lock()
	for _, sess := range c.sessionMap {
		close(sess.done)
	}
	c.sessionMap = make(map[uint64]*rpcSession)
	c.mux.Unlock()
}

func (c *Client) dropMessage(msg Message) {
	if !msg.IsAsync() {
		close(c.deleteSession(msg.Seq()).done)
	} else {
		c.deleteAsyncHandler(msg.Seq())
	}
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

	log.Debug("%v\t%v\trecvLoop start", c.Handler.LogTag(), addr)
	defer log.Debug("%v\t%v\trecvLoop stop", c.Handler.LogTag(), addr)

	if c.Dialer == nil {
		for c.running {
			msg, err = c.Handler.Recv(c)
			if err != nil {
				log.Info("%v\t%v\tDisconnected: %v", c.Handler.LogTag(), addr, err)
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
					log.Info("%v\t%v\tDisconnected: %v", c.Handler.LogTag(), addr, err)
					break
				}
				c.Handler.OnMessage(c, msg)
			}

			c.mux.Lock()
			c.reconnecting = true
			c.mux.Unlock()

			c.Conn.Close()
			c.clearSession()
			c.clearAsyncHandler()

			for c.running {
				log.Info("%v\t%v\tReconnecting ...", c.Handler.LogTag(), addr)
				conn, err := c.Dialer()
				if err == nil {
					c.Conn = conn

					c.initReader()

					c.reconnecting = false

					log.Info("%v\t%v\tReconnected", c.Handler.LogTag(), addr)

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
	log.Debug("%v\t%v\tsendLoop start", c.Handler.LogTag(), addr)
	defer log.Debug("%v\t%v\tsendLoop stop", c.Handler.LogTag(), addr)
	if !c.Handler.BatchSend() {
		var msg Message
		var conn net.Conn
		for {
			select {
			case <-c.chClose:
				return
			case msg = <-c.chSend:
				c.mux.RLock()
				conn = c.Conn
				c.mux.RUnlock()
				if !c.reconnecting {
					if _, err := c.Handler.Send(conn, msg); err != nil {
						conn.Close()
					}
				} else {
					c.dropMessage(msg)
				}
			}
		}
	} else {
		var currLen = 0
		var msg Message
		var conn net.Conn
		var buffers net.Buffers = make([][]byte, 10)[0:0]
		for {
			select {
			case <-c.chClose:
				return
			case msg = <-c.chSend:
			}
			buffers = append(buffers, msg)
			currLen = len(c.chSend)
			for i := 1; i < currLen && i < 10; i++ {
				select {
				case msg = <-c.chSend:
					buffers = append(buffers, msg)
				default:
					goto SEND
				}
			}
		SEND:
			c.mux.RLock()
			conn = c.Conn
			c.mux.RUnlock()
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
			} else {
				for _, v := range buffers {
					c.dropMessage(Message(v))
				}
			}
			buffers = buffers[0:0]
		}
	}
}

func (c *Client) newReqMessage(cmd byte, method string, req interface{}, isAsync bool) Message {
	var (
		data    []byte
		msg     Message
		bodyLen int
	)

	data = util.ValueToBytes(c.Codec, req)

	bodyLen = len(method) + len(data)

	msg = Message(c.Handler.GetBuffer(HeadLen + bodyLen))
	binary.LittleEndian.PutUint32(msg[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(bodyLen))
	binary.LittleEndian.PutUint64(msg[HeaderIndexSeqBegin:HeaderIndexSeqEnd], atomic.AddUint64(&c.seq, 1))
	msg[HeaderIndexCmd] = cmd
	msg.SetAsync(isAsync)
	msg.SetError(false)
	msg[HeaderIndexMethodLen] = byte(len(method))
	copy(msg[HeadLen:HeadLen+len(method)], method)
	copy(msg[HeadLen+len(method):], data)

	return msg
}

// newClientWithConn factory
func newClientWithConn(conn net.Conn, codec codec.Codec, handler Handler, onStop func() int64) *Client {
	log.Info("%v\t%v\tConnected", handler.LogTag(), conn.RemoteAddr())

	c := &Client{}
	c.Conn = conn
	c.Head = Header(c.head[:])
	c.Codec = codec
	c.Handler = handler
	c.chSend = make(chan Message, c.Handler.SendQueueSize())
	c.chClose = make(chan util.Empty)
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]HandlerFunc)
	c.onStop = onStop

	return c
}

// NewClient factory
func NewClient(dialer DialerFunc) (*Client, error) {
	conn, err := dialer()
	if err != nil {
		return nil, err
	}

	c := &Client{}
	c.Conn = conn

	c.Head = Header(c.head[:])
	c.Codec = codec.DefaultCodec
	c.Handler = DefaultHandler.Clone()
	c.Dialer = dialer
	c.chSend = make(chan Message, c.Handler.SendQueueSize())
	c.chClose = make(chan util.Empty)
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]HandlerFunc)

	log.Info("%v\t%v\tConnected", c.Handler.LogTag(), conn.RemoteAddr())

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
	var client = pool.clients[atomic.AddUint64(&pool.round, 1)%pool.size]
	if client.running && !client.reconnecting {
		return client
	}
	for i := uint64(1); i < pool.size; i++ {
		client = pool.clients[atomic.AddUint64(&pool.round, 1)%pool.size]
		if client.running && !client.reconnecting {
			return client
		}
	}
	return client
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
func NewClientPool(dialer DialerFunc, size int) (*ClientPool, error) {
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
		if i > 0 {
			c.Handler = pool.clients[0].Handler
		}
		pool.clients[i] = c
	}

	return pool, nil
}

// NewClientPoolFromDialers factory
func NewClientPoolFromDialers(dialers []DialerFunc) (*ClientPool, error) {
	pool := &ClientPool{
		size:    0,
		round:   0xFFFFFFFFFFFFFFFF,
		clients: []*Client{},
	}

	if len(dialers) == 0 {
		return nil, fmt.Errorf("invalid dialers: empty array")
	}
	var h Handler
	for _, dialer := range dialers {
		c, err := NewClient(dialer)
		if err != nil {
			for j := 0; j < len(pool.clients); j++ {
				pool.clients[j].Stop()
			}
			return nil, err
		}
		if h == nil {
			h = c.Handler
		} else {
			c.Handler = h
		}
		pool.clients = append(pool.clients, c)
	}
	pool.size = uint64(len(pool.clients))

	return pool, nil
}

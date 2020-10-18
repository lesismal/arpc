// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
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
	done chan *Message
}

func newSession(seq uint64) *rpcSession {
	return &rpcSession{seq: seq, done: make(chan *Message, 1)}
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

	chSend  chan *Message
	chClose chan util.Empty

	onStop func(*Client)

	kvmux  sync.RWMutex
	values map[string]interface{}
}

// Get returns value for key
func (c *Client) Get(key string) (interface{}, bool) {
	c.kvmux.RLock()
	defer c.kvmux.RUnlock()
	if len(c.values) == 0 {
		return nil, false
	}
	value, ok := c.values[key]
	return value, ok
}

// Set sets key-value pair
func (c *Client) Set(key string, value interface{}) {
	if value == nil {
		return
	}
	c.kvmux.Lock()
	defer c.kvmux.Unlock()
	if c.values == nil {
		c.values = map[string]interface{}{}
	}
	c.values[key] = value
}

// NewMessage factory
func (c *Client) NewMessage(cmd byte, method string, v interface{}) *Message {
	return newMessage(cmd, method, v, false, false, atomic.AddUint64(&c.seq, 1), c.Handler, c.Codec, nil)
}

// Call make rpc call with timeout
func (c *Client) Call(method string, req interface{}, rsp interface{}, timeout time.Duration) error {
	if err := c.checkCallArgs(method, timeout); err != nil {
		return err
	}

	if timeout < 0 {
		timeout = TimeForever
	}

	timer := time.NewTimer(timeout)

	msg := c.newRequestMessage(CmdRequest, method, req, false, false)
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
	case <-c.chClose:
		c.Handler.OnOverstock(c, msg)
		return ErrClientStopped
	}

	select {
	case msg = <-sess.done:
	case <-timer.C:
		return ErrClientTimeout
	case <-c.chClose:
		return ErrClientStopped
	}

	return c.parseResponse(msg, rsp)
}

func (c *Client) checkCallArgs(method string, timeout time.Duration) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}
	if timeout == 0 {
		return ErrClientInvalidTimeoutZero
	}
	return nil
}

// CallWith make rpc call with context
func (c *Client) CallWith(ctx context.Context, method string, req interface{}, rsp interface{}) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}

	msg := c.newRequestMessage(CmdRequest, method, req, false, false)
	seq := msg.Seq()
	sess := newSession(seq)
	c.addSession(seq, sess)
	defer c.deleteSession(seq)

	select {
	case c.chSend <- msg:
	case <-ctx.Done():
		c.Handler.OnOverstock(c, msg)
		return ErrClientTimeout
	case <-c.chClose:
		c.Handler.OnOverstock(c, msg)
		return ErrClientStopped
	}

	select {
	case msg = <-sess.done:
	case <-ctx.Done():
		return ErrClientTimeout
	case <-c.chClose:
		return ErrClientStopped
	}

	return c.parseResponse(msg, rsp)
}

// CallAsync make async rpc call with timeout
func (c *Client) CallAsync(method string, req interface{}, handler HandlerFunc, timeout time.Duration) error {
	err := c.checkCallAsyncArgs(method, handler, timeout)
	if err != nil {
		return err
	}

	var timer *time.Timer

	msg := c.newRequestMessage(CmdRequest, method, req, false, true)
	seq := msg.Seq()
	if handler != nil {
		c.addAsyncHandler(seq, handler)
		timer = time.AfterFunc(timeout, func() { c.deleteAsyncHandler(seq) })
		defer timer.Stop()
	} else if timeout > 0 {
		timer = time.NewTimer(timeout)
		defer timer.Stop()
	}

	switch timeout {
	case TimeZero:
		err = c.pushMessage(msg, nil)
	default:
		err = c.pushMessage(msg, timer)
	}

	if err != nil && handler != nil {
		c.deleteAsyncHandler(seq)
	}

	return err

}

func (c *Client) checkCallAsyncArgs(method string, handler HandlerFunc, timeout time.Duration) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}
	if timeout < 0 {
		return ErrClientInvalidTimeoutLessThanZero
	}
	if timeout == 0 && handler != nil {
		return ErrClientInvalidTimeoutZeroWithNonNilHandler
	}
	return nil
}

// Notify make rpc notify with timeout
func (c *Client) Notify(method string, data interface{}, timeout time.Duration) error {
	err := c.checkNotifyArgs(method, timeout)
	if err != nil {
		return err
	}

	msg := c.newRequestMessage(CmdNotify, method, data, false, true)
	switch timeout {
	case TimeZero:
		err = c.pushMessage(msg, nil)
	default:
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		err = c.pushMessage(msg, timer)
	}

	return err
}

func (c *Client) checkNotifyArgs(method string, timeout time.Duration) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}
	if timeout < 0 {
		return ErrClientInvalidTimeoutLessThanZero
	}
	return nil
}

// NotifyWith make rpc notify with context
func (c *Client) NotifyWith(ctx context.Context, method string, data interface{}) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}

	msg := c.newRequestMessage(CmdNotify, method, data, false, true)

	select {
	case c.chSend <- msg:
	case <-ctx.Done():
		c.Handler.OnOverstock(c, msg)
		return ErrClientTimeout
	case <-c.chClose:
		c.Handler.OnOverstock(c, msg)
		return ErrClientStopped
	}

	return nil
}

// PushMsg push msg to client's send queue
func (c *Client) PushMsg(msg *Message, timeout time.Duration) error {
	err := c.checkState()
	if err != nil {
		return err
	}

	if timeout < 0 {
		timeout = TimeForever
	}

	switch timeout {
	case TimeZero:
		select {
		case c.chSend <- msg:
		default:
			c.Handler.OnOverstock(c, msg)
			return ErrClientOverstock
		}
	case TimeForever:
		select {
		case c.chSend <- msg:
		case <-c.chClose:
			c.Handler.OnOverstock(c, msg)
			return ErrClientStopped
		}
	default:
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		err = c.pushMessage(msg, timer)
	}

	return err
}

func (c *Client) pushMessage(msg *Message, timer *time.Timer) error {
	if timer == nil {
		select {
		case c.chSend <- msg:
		case <-c.chClose:
			c.Handler.OnOverstock(c, msg)
			return ErrClientStopped
		default:
			c.Handler.OnOverstock(c, msg)
			return ErrClientOverstock
		}
	} else {
		select {
		case c.chSend <- msg:
		case <-timer.C:
			c.Handler.OnOverstock(c, msg)
			return ErrClientTimeout
		case <-c.chClose:
			c.Handler.OnOverstock(c, msg)
			return ErrClientStopped
		}
	}
	return nil
}

func (c *Client) checkState() error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}
	return nil
}

func (c *Client) checkStateAndMethod(method string) error {
	err := c.checkState()
	if err != nil {
		return err
	}
	return checkMethod(method)
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
			c.onStop(c)
		}
		c.Handler.OnDisconnected(c)
	}
}

func (c *Client) newRequestMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool) *Message {
	return newMessage(cmd, method, v, isError, isAsync, atomic.AddUint64(&c.seq, 1), c.Handler, c.Codec, nil)
}

func (c *Client) parseResponse(msg *Message, rsp interface{}) error {
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
				*vt = string(msg.Data())
			case *[]byte:
				*vt = msg.Data()
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

func (c *Client) dropMessage(msg *Message) {
	if !msg.IsAsync() {
		session := c.deleteSession(msg.Seq())
		if session != nil {
			close(session.done)
		}
		c.Handler.OnMessageDropped(c, msg)
	} else {
		c.deleteAsyncHandler(msg.Seq())
		c.Handler.OnMessageDropped(c, msg)
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

// Restart stop and restarts a client
func (c *Client) Restart() error {
	c.Stop()

	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		conn, err := c.Dialer()
		if err != nil {
			return err
		}

		preConn := c.Conn
		c.Conn = conn

		c.chSend = make(chan *Message, c.Handler.SendQueueSize())
		c.chClose = make(chan util.Empty)
		c.sessionMap = make(map[uint64]*rpcSession)
		c.asyncHandlerMap = make(map[uint64]HandlerFunc)

		c.initReader()
		go util.Safe(c.sendLoop)
		go util.Safe(c.recvLoop)

		c.running = true
		c.reconnecting = false

		log.Info("%v\t[%v] Restarted to [%v]", c.Handler.LogTag(), preConn.RemoteAddr(), conn.RemoteAddr())
	}

	return nil
}

func (c *Client) run() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		go util.Safe(c.sendLoop)
		go util.Safe(c.recvLoop)
	}
}

func (c *Client) runWebsocket() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		go util.Safe(c.sendLoop)
		c.Conn.(WebsocketConn).HandleWebsocket(c.recvLoop)
	}
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
		msg  *Message
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

			c.reconnecting = true

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

	if c.Handler.BatchSend() {
		c.batchSendLoop()
	} else {
		c.normalSendLoop()
	}
}

func (c *Client) normalSendLoop() {
	var msg *Message
	var coders = c.Handler.Coders()
	for {
		select {
		case msg = <-c.chSend:
			if !c.reconnecting {
				for j := 0; j < len(coders); j++ {
					msg = coders[j].Encode(c, msg)
				}
				if _, err := c.Handler.Send(c.Conn, msg.Buffer); err != nil {
					c.Conn.Close()
				}
			} else {
				c.dropMessage(msg)
			}
		case <-c.chClose:
			return
		}
	}
}

func (c *Client) batchSendLoop() {
	var msg *Message
	var coders = c.Handler.Coders()
	var messages []*Message = make([]*Message, 10)[0:0]
	var buffers net.Buffers = make([][]byte, 10)[0:0]
	for {
		select {
		case msg = <-c.chSend:
		case <-c.chClose:
			return
		}
		messages = append(messages, msg)
		for i := 1; i < len(c.chSend) && i < 10; i++ {
			msg = <-c.chSend
			messages = append(messages, msg)
		}
		if !c.reconnecting {
			if len(messages) == 1 {
				for j := 0; j < len(coders); j++ {
					messages[0] = coders[j].Encode(c, messages[0])
				}
				if _, err := c.Handler.Send(c.Conn, messages[0].Buffer); err != nil {
					c.Conn.Close()
				}
			} else {
				for i := 0; i < len(messages); i++ {
					for j := 0; j < len(coders); j++ {
						messages[i] = coders[j].Encode(c, messages[i])
					}
					buffers = append(buffers, messages[i].Buffer)
				}
				if _, err := c.Handler.SendN(c.Conn, buffers); err != nil {
					c.Conn.Close()
				}
				buffers = buffers[0:0]
			}
		} else {
			for _, m := range messages {
				c.dropMessage(m)
			}
		}
		messages = messages[0:0]
	}
}

// newClientWithConn factory
func newClientWithConn(conn net.Conn, codec codec.Codec, handler Handler, onStop func(*Client)) *Client {
	log.Info("%v\t%v\tConnected", handler.LogTag(), conn.RemoteAddr())

	c := &Client{}
	c.Conn = conn
	c.Head = Header(c.head[:])
	c.Codec = codec
	c.Handler = handler
	c.chSend = make(chan *Message, c.Handler.SendQueueSize())
	c.chClose = make(chan util.Empty)
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]HandlerFunc)
	c.onStop = onStop

	if _, ok := conn.(WebsocketConn); !ok {
		c.run()
	} else {
		c.runWebsocket()
	}

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
	c.chSend = make(chan *Message, c.Handler.SendQueueSize())
	c.chClose = make(chan util.Empty)
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]HandlerFunc)

	c.run()

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
		return nil, ErrClientInvalidPoolDialers
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

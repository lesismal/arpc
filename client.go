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
	// TimeZero represents zero time.
	TimeZero time.Duration = 0
	// TimeForever represents forever time.
	TimeForever time.Duration = 1<<63 - 1
)

// DialerFunc defines the dialer used by arpc Client to connect to the server.
type DialerFunc func() (net.Conn, error)

// rpcSession represents an active calling session.
type rpcSession struct {
	seq  uint64
	done chan *Message
}

// newSession creates rpcSession
func newSession(seq uint64) *rpcSession {
	return &rpcSession{seq: seq, done: make(chan *Message, 1)}
}

// Client represents an arpc Client.
// There may be multiple outstanding Calls or Notifys associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
type Client struct {
	// 64-aligned on 32-bit
	seq uint64

	Conn    net.Conn
	Codec   codec.Codec
	Handler Handler
	Reader  io.Reader
	Dialer  DialerFunc
	Head    Header

	running      bool
	reconnecting bool

	mux             sync.Mutex
	sessionMap      map[uint64]*rpcSession
	asyncHandlerMap map[uint64]HandlerFunc

	chSend  chan *Message
	chClose chan util.Empty

	onStop func(*Client)

	values map[interface{}]interface{}
	// UserData interface{}
}

// Get returns value for key.
func (c *Client) Get(key interface{}) (interface{}, bool) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if len(c.values) == 0 {
		return nil, false
	}
	value, ok := c.values[key]
	return value, ok
}

// Set sets key-value pair.
func (c *Client) Set(key interface{}, value interface{}) {
	if key == nil || value == nil {
		return
	}
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.running {
		if c.values == nil {
			c.values = map[interface{}]interface{}{}
		}
		c.values[key] = value
	}
}

// Delete deletes key-value pair
func (c *Client) Delete(key interface{}) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.running && c.values != nil {
		delete(c.values, key)
	}
}

// NewMessage creates a Message by client's seq, handler and codec.
func (c *Client) NewMessage(cmd byte, method string, v interface{}, args ...interface{}) *Message {
	if len(args) == 0 {
		return newMessage(cmd, method, v, false, false, atomic.AddUint64(&c.seq, 1), c.Handler, c.Codec, nil)
	}
	return newMessage(cmd, method, v, false, false, atomic.AddUint64(&c.seq, 1), c.Handler, c.Codec, args[0].(map[interface{}]interface{}))
}

// Call makes an rpc call with a timeout.
// Call will block waiting for the server's response until timeout.
func (c *Client) Call(method string, req interface{}, rsp interface{}, timeout time.Duration, args ...interface{}) error {
	if err := c.checkCallArgs(method, timeout); err != nil {
		return err
	}

	// if timeout < 0 {
	// 	timeout = TimeForever
	// }

	timer := time.NewTimer(timeout)

	msg := c.newRequestMessage(CmdRequest, method, req, false, false, args...)
	seq := msg.Seq()
	sess := newSession(seq)
	c.addSession(seq, sess)
	defer func() {
		timer.Stop()
		c.deleteSession(seq)
	}()

	if c.Handler.AsyncWrite() {
		select {
		case c.chSend <- msg:
		case <-timer.C:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientTimeout
		case <-c.chClose:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientStopped
		}
	} else {
		if !c.reconnecting {
			coders := c.Handler.Coders()
			for j := 0; j < len(coders); j++ {
				msg = coders[j].Encode(c, msg)
			}
			_, err := c.Handler.Send(c.Conn, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			}
			c.Handler.OnMessageDone(c, msg)
			return err
		} else {
			c.dropMessage(msg)
			return ErrClientReconnecting
		}
	}

	select {
	case msg = <-sess.done:
	case <-timer.C:
		return ErrClientTimeout
	case <-c.chClose:
		return ErrClientStopped
	}

	err := c.parseResponse(msg, rsp)
	c.Handler.OnMessageDone(c, msg)
	return err
}

// CallWith uses context to make rpc call.
// CallWith blocks to wait for a response from the server until it times out.
func (c *Client) CallWith(ctx context.Context, method string, req interface{}, rsp interface{}, args ...interface{}) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}

	msg := c.newRequestMessage(CmdRequest, method, req, false, false, args...)
	seq := msg.Seq()
	sess := newSession(seq)
	c.addSession(seq, sess)
	defer c.deleteSession(seq)

	if c.Handler.AsyncWrite() {
		select {
		case c.chSend <- msg:
		case <-ctx.Done():
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientTimeout
		case <-c.chClose:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientStopped
		}
	} else {
		if !c.reconnecting {
			coders := c.Handler.Coders()
			for j := 0; j < len(coders); j++ {
				msg = coders[j].Encode(c, msg)
			}
			_, err := c.Handler.Send(c.Conn, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			}
			c.Handler.OnMessageDone(c, msg)
			return err
		} else {
			c.dropMessage(msg)
			return ErrClientReconnecting
		}
	}

	select {
	case msg = <-sess.done:
	case <-ctx.Done():
		return ErrClientTimeout
	case <-c.chClose:
		return ErrClientStopped
	}

	err := c.parseResponse(msg, rsp)
	c.Handler.OnMessageDone(c, msg)
	return err
}

// CallAsync makes an asynchronous rpc call with timeout.
// CallAsync will not block waiting for the server's response,
// But the handler will be called if the response arrives before the timeout.
func (c *Client) CallAsync(method string, req interface{}, handler HandlerFunc, timeout time.Duration, args ...interface{}) error {
	err := c.checkCallAsyncArgs(method, handler, timeout)
	if err != nil {
		return err
	}

	var timer *time.Timer

	msg := c.newRequestMessage(CmdRequest, method, req, false, true, args...)
	seq := msg.Seq()
	if handler != nil {
		c.addAsyncHandler(seq, handler)
		timer = time.AfterFunc(timeout, func() { c.deleteAsyncHandler(seq) })
		defer timer.Stop()
	} else if timeout > 0 {
		timer = time.NewTimer(timeout)
		defer timer.Stop()
	}

	if c.Handler.AsyncWrite() {
		switch timeout {
		case TimeZero:
			err = c.pushMessage(msg, nil)
		default:
			err = c.pushMessage(msg, timer)
		}
	} else {
		if !c.reconnecting {
			coders := c.Handler.Coders()
			for j := 0; j < len(coders); j++ {
				msg = coders[j].Encode(c, msg)
			}
			_, err = c.Handler.Send(c.Conn, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			}
			c.Handler.OnMessageDone(c, msg)
		} else {
			c.dropMessage(msg)
			err = ErrClientReconnecting
		}
	}

	if err != nil && handler != nil {
		c.deleteAsyncHandler(seq)
	}

	return err
}

// Notify makes a notify with timeout.
// A notify does not need a response from the server.
func (c *Client) Notify(method string, data interface{}, timeout time.Duration, args ...interface{}) error {
	err := c.checkNotifyArgs(method, timeout)
	if err != nil {
		return err
	}

	msg := c.newRequestMessage(CmdNotify, method, data, false, true, args...)

	if c.Handler.AsyncWrite() {
		switch timeout {
		case TimeZero:
			err = c.pushMessage(msg, nil)
		default:
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			err = c.pushMessage(msg, timer)
		}
	} else {
		if !c.reconnecting {
			coders := c.Handler.Coders()
			for j := 0; j < len(coders); j++ {
				msg = coders[j].Encode(c, msg)
			}
			_, err = c.Handler.Send(c.Conn, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			}
			c.Handler.OnMessageDone(c, msg)
		} else {
			c.dropMessage(msg)
			err = ErrClientReconnecting
		}
	}

	return err
}

// NotifyWith use context to make rpc notify.
// A notify does not need a response from the server.
func (c *Client) NotifyWith(ctx context.Context, method string, data interface{}, args ...interface{}) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}

	msg := c.newRequestMessage(CmdNotify, method, data, false, true, args...)

	if c.Handler.AsyncWrite() {
		select {
		case c.chSend <- msg:
		case <-ctx.Done():
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientTimeout
		case <-c.chClose:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientStopped
		}
	} else {
		if !c.reconnecting {
			coders := c.Handler.Coders()
			for j := 0; j < len(coders); j++ {
				msg = coders[j].Encode(c, msg)
			}
			_, err := c.Handler.Send(c.Conn, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			}
			c.Handler.OnMessageDone(c, msg)
			return err
		} else {
			c.dropMessage(msg)
			return ErrClientReconnecting
		}
	}

	return nil
}

// PushMsg pushes a msg to Client's send queue with timeout.
func (c *Client) PushMsg(msg *Message, timeout time.Duration) error {
	err := c.CheckState()
	if err != nil {
		c.Handler.OnMessageDone(c, msg)
		return err
	}

	if !c.Handler.AsyncWrite() {
		if !c.reconnecting {
			coders := c.Handler.Coders()
			for j := 0; j < len(coders); j++ {
				msg = coders[j].Encode(c, msg)
			}
			_, err := c.Handler.Send(c.Conn, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			}
			c.Handler.OnMessageDone(c, msg)
			return err
		} else {
			c.dropMessage(msg)
			return ErrClientReconnecting
		}
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
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientStopped
		}
	default:
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		err = c.pushMessage(msg, timer)
	}

	return err
}

// Restart stops and restarts a Client.
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
		c.values = map[interface{}]interface{}{}

		c.initReader()
		if c.Handler.AsyncWrite() {
			go util.Safe(c.sendLoop)
		}
		go util.Safe(c.recvLoop)

		c.running = true
		c.reconnecting = false

		log.Info("%v\t[%v] Restarted to [%v]", c.Handler.LogTag(), preConn.RemoteAddr(), conn.RemoteAddr())
	}

	return nil
}

// Stop stops a Client.
func (c *Client) Stop() {
	c.mux.Lock()
	c.running = false
	c.mux.Unlock()

	c.Conn.Close()
}

func (c *Client) closeAndClean() {
	c.mux.Lock()
	c.running = false
	c.mux.Unlock()

	c.Conn.Close()
	if c.chSend != nil {
		close(c.chClose)
	}
	if c.onStop != nil {
		c.onStop(c)
	}
	c.Handler.OnDisconnected(c)
}

// CheckState checks Client's state.
func (c *Client) CheckState() error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}
	return nil
}

func (c *Client) checkCallArgs(method string, timeout time.Duration) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}
	if timeout == 0 {
		return ErrClientInvalidTimeoutZero
	}
	if timeout < 0 {
		return ErrClientInvalidTimeoutLessThanZero
	}
	return nil
}

func (c *Client) checkCallAsyncArgs(method string, handler HandlerFunc, timeout time.Duration) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}

	if timeout < 0 {
		return ErrClientInvalidTimeoutLessThanZero
	}
	if timeout == 0 && handler != nil {
		return ErrClientInvalidTimeoutZeroWithNonNilCallback
	}
	return nil
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

func (c *Client) checkStateAndMethod(method string) error {
	err := c.CheckState()
	if err != nil {
		return err
	}
	return checkMethod(method)
}

func (c *Client) pushMessage(msg *Message, timer *time.Timer) error {
	if timer == nil {
		select {
		case c.chSend <- msg:
		case <-c.chClose:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientStopped
		default:
			c.Handler.OnOverstock(c, msg)
			return ErrClientOverstock
		}
	} else {
		select {
		case c.chSend <- msg:
		case <-timer.C:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientTimeout
		case <-c.chClose:
			c.Handler.OnMessageDone(c, msg)
			// c.Handler.OnOverstock(c, msg)
			return ErrClientStopped
		}
	}
	return nil
}

func (c *Client) newRequestMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, args ...interface{}) *Message {
	if len(args) == 0 {
		return newMessage(cmd, method, v, isError, isAsync, atomic.AddUint64(&c.seq, 1), c.Handler, c.Codec, nil)
	}
	return newMessage(cmd, method, v, isError, isAsync, atomic.AddUint64(&c.seq, 1), c.Handler, c.Codec, args[0].(map[interface{}]interface{}))
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
				*vt = append([]byte{}, msg.Data()...)
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
	if c.running {
		c.sessionMap[seq] = session
	}
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
	if c.running {
		c.asyncHandlerMap[seq] = h
	}
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

func (c *Client) run() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		if c.Handler.AsyncWrite() {
			go util.Safe(c.sendLoop)
		}
		go util.Safe(c.recvLoop)
	}
}

func (c *Client) runWebsocket() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		if c.Handler.AsyncWrite() {
			go util.Safe(c.sendLoop)
		}
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
				log.Error("%v\t%v\tDisconnected: %v", c.Handler.LogTag(), addr, err)
				c.closeAndClean()
				return
			}
			c.Handler.OnMessage(c, msg)
		}
	} else {
		c.Handler.OnConnected(c)

		for c.running {
		RECV:
			for {
				msg, err = c.Handler.Recv(c)
				if err != nil {
					log.Error("%v\t%v\tDisconnected: %v", c.Handler.LogTag(), addr, err)
					break
				}
				c.Handler.OnMessage(c, msg)
			}

			c.reconnecting = true

			c.Conn.Close()
			c.clearSession()
			c.clearAsyncHandler()

			// if c.running {
			// 	log.Info("%v\t%v\tReconnect Start", c.Handler.LogTag(), addr)
			// }
			maxReconnectTimes := c.Handler.MaxReconnectTimes()
			for i := 0; c.running && ((maxReconnectTimes <= 0) || (i < maxReconnectTimes)); i++ {
				log.Info("%v\t%v\tReconnect Trying %v", c.Handler.LogTag(), addr, i)
				conn, err := c.Dialer()
				if err == nil {
					c.Conn = conn

					c.initReader()

					c.reconnecting = false

					log.Info("%v\t%v\tReconnected", c.Handler.LogTag(), addr)

					go c.Handler.OnConnected(c)

					goto RECV
				}

				time.Sleep(time.Second)
			}
			c.closeAndClean()
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
	var coders []MessageCoder
	for {
		select {
		case msg = <-c.chSend:
			if !c.reconnecting {
				coders = c.Handler.Coders()
				for j := 0; j < len(coders); j++ {
					msg = coders[j].Encode(c, msg)
				}
				if _, err := c.Handler.Send(c.Conn, msg.Buffer); err != nil {
					c.Conn.Close()
				}
				c.Handler.OnMessageDone(c, msg)
			} else {
				c.dropMessage(msg)
			}
		case <-c.chClose:
			// clear msg in send queue
			for {
				select {
				case msg := <-c.chSend:
					c.Handler.OnMessageDone(c, msg)
				default:
					return
				}
			}
		}
	}
}

func (c *Client) batchSendLoop() {
	var msg *Message
	var chLen int
	var coders []MessageCoder
	var buffer = c.Handler.Malloc(2048)[0:0]
	var sendBufferSize = c.Handler.SendBufferSize()
	defer c.Handler.Free(buffer)

	for {
		select {
		case msg = <-c.chSend:
		case <-c.chClose:
			// clear msg in send queue
			for {
				select {
				case msg := <-c.chSend:
					c.Handler.OnMessageDone(c, msg)
				default:
					return
				}
			}
		}

		if !c.reconnecting {
			chLen = len(c.chSend)
			coders = c.Handler.Coders()
			for i := 0; i < chLen && len(buffer) < sendBufferSize; i++ {
				if len(buffer) == 0 {
					for j := 0; j < len(coders); j++ {
						msg = coders[j].Encode(c, msg)
					}
					buffer = append(buffer, msg.Buffer...)
					c.Handler.OnMessageDone(c, msg)
				}
				msg = <-c.chSend
				for j := 0; j < len(coders); j++ {
					msg = coders[j].Encode(c, msg)
				}
				buffer = append(buffer, msg.Buffer...)
				c.Handler.OnMessageDone(c, msg)
			}
			if len(buffer) == 0 {
				for j := 0; j < len(coders); j++ {
					msg = coders[j].Encode(c, msg)
				}
				_, err := c.Handler.Send(c.Conn, msg.Buffer)
				if err != nil {
					c.Conn.Close()
				}
				c.Handler.OnMessageDone(c, msg)
			} else {
				if _, err := c.Handler.Send(c.Conn, buffer); err != nil {
					c.Conn.Close()
				}
				buffer = buffer[0:0]
			}
		} else {
			c.dropMessage(msg)
		}
	}
}

func newClientWithConn(conn net.Conn, codec codec.Codec, handler Handler, onStop func(*Client)) *Client {
	log.Info("%v\t%v\tConnected", handler.LogTag(), conn.RemoteAddr())

	c := &Client{}
	c.Conn = conn
	c.Codec = codec
	c.Handler = handler
	c.Head = make([]byte, 4)
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

// NewClient creates a Client.
func NewClient(dialer DialerFunc, args ...interface{}) (*Client, error) {
	conn, err := dialer()
	if err != nil {
		return nil, err
	}

	var handler Handler
	if len(args) > 0 {
		if h, ok := args[0].(Handler); ok {
			handler = h
		}
	}
	if handler == nil {
		handler = DefaultHandler.Clone()
	}

	c := &Client{}
	c.Conn = conn
	c.Codec = codec.DefaultCodec
	c.Handler = handler
	c.Dialer = dialer
	c.Head = make([]byte, 4)
	c.chSend = make(chan *Message, c.Handler.SendQueueSize())
	c.chClose = make(chan util.Empty)
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]HandlerFunc)

	c.run()

	log.Info("%v\t%v\tConnected", c.Handler.LogTag(), conn.RemoteAddr())

	return c, nil
}

// ClientPool represents an arpc Client Pool.
type ClientPool struct {
	size    uint64
	round   uint64
	clients []*Client
}

// Size returns Client number.
func (pool *ClientPool) Size() int {
	return len(pool.clients)
}

// Get returns a Client by index.
func (pool *ClientPool) Get(index int) *Client {
	return pool.clients[uint64(index)%pool.size]
}

// Next returns a Client by round robin.
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

// Handler returns Handler.
func (pool *ClientPool) Handler() Handler {
	return pool.Next().Handler
}

// Stop stops all clients.
func (pool *ClientPool) Stop() {
	for _, c := range pool.clients {
		c.Stop()
	}
}

// NewClientPool creates a ClientPool.
func NewClientPool(dialer DialerFunc, size int, args ...interface{}) (*ClientPool, error) {
	pool := &ClientPool{
		size:    uint64(size),
		round:   0xFFFFFFFFFFFFFFFF,
		clients: make([]*Client, size),
	}

	var handler Handler
	if len(args) > 0 {
		if h, ok := args[0].(Handler); ok {
			handler = h
		}
	}
	if handler == nil {
		handler = DefaultHandler.Clone()
	}

	for i := 0; i < size; i++ {
		c, err := NewClient(dialer, handler)
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

// NewClientPoolFromDialers creates a ClientPool by multiple dialers.
func NewClientPoolFromDialers(dialers []DialerFunc, args ...interface{}) (*ClientPool, error) {
	pool := &ClientPool{
		size:    0,
		round:   0xFFFFFFFFFFFFFFFF,
		clients: []*Client{},
	}

	if len(dialers) == 0 {
		return nil, ErrClientInvalidPoolDialers
	}

	var handler Handler
	if len(args) > 0 {
		if h, ok := args[0].(Handler); ok {
			handler = h
		}
	}
	if handler == nil {
		handler = DefaultHandler.Clone()
	}

	for _, dialer := range dialers {
		c, err := NewClient(dialer, handler)
		if err != nil {
			for j := 0; j < len(pool.clients); j++ {
				pool.clients[j].Stop()
			}
			return nil, err
		}
		pool.clients = append(pool.clients, c)
	}
	pool.size = uint64(len(pool.clients))

	return pool, nil
}

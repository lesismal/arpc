// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bufio"
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

var (
	emptyAsyncHandler = asyncHandler{}
	asyncHandlerPool  = sync.Pool{
		New: func() interface{} {
			return &asyncHandler{}
		},
	}
)

type asyncHandler struct {
	h HandlerFunc
	t *time.Timer
}

func getAsyncHandler() *asyncHandler {
	return asyncHandlerPool.Get().(*asyncHandler)
}

func putAsyncHandler(h *asyncHandler) {
	*h = emptyAsyncHandler
	asyncHandlerPool.Put(h)
}

// Client represents an arpc Client.
// There may be multiple outstanding Calls or Notifys associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
type Client struct {
	// 64-aligned on 32-bit
	seq uint64

	Conn    net.Conn
	Writer  *bufio.Writer
	Codec   codec.Codec
	Handler Handler
	Reader  io.Reader
	Dialer  DialerFunc
	Head    Header

	running      bool
	reconnecting bool

	mux             sync.Mutex
	sessionMap      map[uint64]*rpcSession
	asyncHandlerMap map[uint64]*asyncHandler
	sendMux         sync.Mutex
	sendQueue       []*Message
	// chSend    chan *Message
	sendQueueHead *job
	sendQueueTail *job

	chClose chan util.Empty

	onStop func(*Client)

	values map[interface{}]interface{}
	// UserData interface{}
}

// SetState sets running state, should be used only for non-blocking conn.
func (c *Client) SetState(running bool) {
	c.running = running
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
	if c.values == nil {
		c.values = map[interface{}]interface{}{}
	}
	c.values[key] = value
	c.mux.Unlock()
}

// Delete deletes key-value pair
func (c *Client) Delete(key interface{}) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.values != nil {
		delete(c.values, key)
	}
}

// Ping .
func (c *Client) Ping() {
	c.Conn.Write(PingMessage.Buffer)
}

// Pong .
func (c *Client) Pong() {
	c.Conn.Write(PongMessage.Buffer)
}

// Ping .
func (c *Client) Keepalive(interval time.Duration) {
	if c.running {
		if interval <= 0 {
			interval = time.Second * 30
		}
		time.AfterFunc(interval, func() {
			c.Ping()
			c.Keepalive(interval)
		})
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

	coders := c.Handler.Coders()
	for j := 0; j < len(coders); j++ {
		msg = coders[j].Encode(c, msg)
	}

	if c.Handler.AsyncWrite() {
		err := c.sendAsync(msg)
		if err != nil {
			return err
		}
	} else {
		if !c.reconnecting {
			_, err := c.Handler.Send(c.Conn, c.Writer, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			} else {
				c.Writer.Flush()
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

	coders := c.Handler.Coders()
	for j := 0; j < len(coders); j++ {
		msg = coders[j].Encode(c, msg)
	}

	if c.Handler.AsyncWrite() {
		err := c.sendAsync(msg)
		if err != nil {
			return err
		}
	} else {
		if !c.reconnecting {
			_, err := c.Handler.Send(c.Conn, c.Writer, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			} else {
				c.Writer.Flush()
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

	msg := c.newRequestMessage(CmdRequest, method, req, false, true, args...)
	seq := msg.Seq()
	coders := c.Handler.Coders()
	for j := 0; j < len(coders); j++ {
		msg = coders[j].Encode(c, msg)
	}

	var timer *time.Timer
	if handler != nil {
		if timeout <= 0 {
			timeout = time.Second * 10
		}
		timer = time.AfterFunc(timeout, func() { c.deleteAsyncHandler(seq) })
		c.addAsyncHandler(seq, handler, timer)
	}

	if c.Handler.AsyncWrite() {
		err = c.sendAsync(msg)
	} else {
		if !c.reconnecting {
			_, err = c.Handler.Send(c.Conn, c.Writer, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			} else {
				c.Writer.Flush()
			}
			c.Handler.OnMessageDone(c, msg)
		} else {
			c.dropMessage(msg)
			err = ErrClientReconnecting
		}
	}

	if err != nil {
		if handler != nil {
			c.deleteAsyncHandler(seq)
		}
		if timer != nil {
			timer.Stop()
		}
	}

	return err
}

// Notify makes a notify with timeout.
// A notify does not need a response from the server.
func (c *Client) Notify(method string, data interface{}, timeout time.Duration, args ...interface{}) error {
	return c.NotifyWith(nil, method, data, args...)
}

func (c *Client) NotifyContext(ctx context.Context, method string, data interface{}, args ...interface{}) error {
	return c.NotifyWith(ctx, method, data, args...)
}

// NotifyWith use context to make rpc notify.
// A notify does not need a response from the server.
func (c *Client) NotifyWith(ctx context.Context, method string, data interface{}, args ...interface{}) error {
	err := c.checkStateAndMethod(method)
	if err != nil {
		return err
	}

	if ctx != nil {
		select {
		case <-ctx.Done():
			return ErrClientTimeout
		default:
		}
	}

	msg := c.newRequestMessage(CmdNotify, method, data, false, true, args...)
	coders := c.Handler.Coders()
	for j := 0; j < len(coders); j++ {
		msg = coders[j].Encode(c, msg)
	}

	if c.Handler.AsyncWrite() {
		return c.sendAsync(msg)
	}

	if !c.reconnecting {
		_, err := c.Handler.Send(c.Conn, c.Writer, msg.Buffer)
		if err != nil {
			c.Conn.Close()
		} else {
			c.Writer.Flush()
		}
		c.Handler.OnMessageDone(c, msg)
		return err
	}

	c.dropMessage(msg)
	return ErrClientReconnecting
}

// PushMsg pushes a msg to Client's send queue with timeout.
func (c *Client) PushMsg(msg *Message, timeout time.Duration) error {
	err := c.CheckState()
	if err != nil {
		c.Handler.OnMessageDone(c, msg)
		return err
	}

	coders := c.Handler.Coders()
	for j := 0; j < len(coders); j++ {
		msg = coders[j].Encode(c, msg)
	}

	if c.Handler.AsyncWrite() {
		return c.sendAsync(msg)
	}

	if !c.reconnecting {
		c.dropMessage(msg)
		return ErrClientReconnecting
	}
	_, err = c.Handler.Send(c.Conn, c.Writer, msg.Buffer)
	if err != nil {
		c.Conn.Close()
	} else {
		c.Writer.Flush()
	}
	c.Handler.OnMessageDone(c, msg)
	return err
}

// // Restart stops and restarts a Client.
// func (c *Client) Restart() error {
// 	c.Stop()

// 	c.mux.Lock()
// 	defer c.mux.Unlock()
// 	if !c.running {
// 		conn, err := c.Dialer()
// 		if err != nil {
// 			return err
// 		}

// 		preConn := c.Conn
// 		c.Conn = conn

// 		// c.chSend = make(chan *Message, c.Handler.SendQueueSize())
// 		c.chClose = make(chan util.Empty)
// 		c.sessionMap = make(map[uint64]*rpcSession)
// 		c.asyncHandlerMap = make(map[uint64]*asyncHandler)
// 		c.values = map[interface{}]interface{}{}

// 		c.initReader()
// 		// if c.Handler.AsyncWrite() {
// 		// 	go util.Safe(c.sendLoop)
// 		// }
// 		go util.Safe(c.recvLoop)

// 		c.running = true
// 		c.reconnecting = false

// 		log.Info("%v\t[%v] Restarted to [%v]", c.Handler.LogTag(), preConn.RemoteAddr(), conn.RemoteAddr())
// 	}

// 	return nil
// }

func (c *Client) sendAsync(msg *Message) error {
	if c.reconnecting {
		c.Handler.OnMessageDone(c, msg)
		return ErrClientReconnecting
	}
	if !c.running {
		c.Handler.OnMessageDone(c, msg)
		return ErrClientStopped
	}

	if msg == nil {
		return nil
	}

	jo := getJob()
	jo.msg = msg

	c.mux.Lock()
	if c.sendQueueHead != nil {
		c.sendQueueTail.next = jo
		c.sendQueueTail = jo
		c.mux.Unlock()
		return nil
	}

	c.sendQueueHead = jo
	c.sendQueueTail = jo
	c.mux.Unlock()

	go func() {
		defer c.Writer.Flush()

		if bfSend := c.Handler.BeforeSendHandler(); bfSend != nil {
			if err := bfSend(c.Conn); err != nil {
				return
			}
		}
		if writeTimeout := c.Handler.WriteTimeout(); writeTimeout > 0 {
			c.Conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		}
		next := jo
		for {
			msg = next.msg
			c.Handler.Send(c.Conn, c.Writer, msg.Buffer)
			c.Handler.OnMessageDone(c, msg)
			c.mux.Lock()
			if !c.running {
				next = c.sendQueueHead
				for next != nil {
					putJob(next)
					next = next.next
				}
				c.sendQueueHead = nil
				c.sendQueueTail = nil
				return
			}
			c.sendQueueHead = next.next
			putJob(next)
			next = c.sendQueueHead
			if next == nil {
				c.sendQueueTail = nil
				c.mux.Unlock()
				return
			}
			c.mux.Unlock()
		}
	}()

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
	// if c.chSend != nil {
	// 	close(c.chClose)
	// }
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

func (c *Client) pushMessage(msg *Message) error {
	c.sendMux.Lock()
	defer c.sendMux.Unlock()

	if len(c.sendQueue) >= c.Handler.SendQueueSize() {
		c.Handler.OnOverstock(c, msg)
		return ErrClientOverstock
	}

	c.sendQueue = append(c.sendQueue, msg)

	head := c.sendQueue[0]
	if head == msg {
		go func() {

		}()
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

func (c *Client) addAsyncHandler(seq uint64, h HandlerFunc, t *time.Timer) {
	c.mux.Lock()
	if c.running {
		ah := getAsyncHandler()
		ah.h = h
		ah.t = t
		c.asyncHandlerMap[seq] = ah
	}
	c.mux.Unlock()
}

func (c *Client) deleteAsyncHandler(seq uint64) {
	c.mux.Lock()
	delete(c.asyncHandlerMap, seq)
	c.mux.Unlock()
}

func (c *Client) getAndDeleteAsyncHandler(seq uint64) (*asyncHandler, bool) {
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
	c.asyncHandlerMap = make(map[uint64]*asyncHandler)
	c.mux.Unlock()
}

func (c *Client) clearSendQueue() {
	c.mux.Lock()
	next := c.sendQueueHead
	for next != nil {
		putJob(next)
		next = next.next
	}
	c.sendQueueHead = nil
	c.sendQueueTail = nil
	c.mux.Unlock()
}

func (c *Client) run() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		// if c.Handler.AsyncWrite() {
		// 	go util.Safe(c.sendLoop)
		// }
		go util.Safe(c.recvLoop)
	}
}

func (c *Client) runWebsocket() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		// if c.Handler.AsyncWrite() {
		// 	go util.Safe(c.sendLoop)
		// }
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
		go c.Handler.OnConnected(c)

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
			c.clearSendQueue()

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

// func (c *Client) sendLoop() {
// 	addr := c.Conn.RemoteAddr().String()
// 	log.Debug("%v\t%v\tsendLoop start", c.Handler.LogTag(), addr)
// 	defer log.Debug("%v\t%v\tsendLoop stop", c.Handler.LogTag(), addr)

// 	if c.Handler.BatchSend() {
// 		c.batchSendLoop()
// 	} else {
// 		c.normalSendLoop()
// 	}
// }

// func (c *Client) normalSendLoop() {
// 	var msg *Message
// 	var coders []MessageCoder
// 	for {
// 		select {
// 		case msg = <-c.chSend:
// 			if !c.reconnecting {
// 				coders = c.Handler.Coders()
// 				for j := 0; j < len(coders); j++ {
// 					msg = coders[j].Encode(c, msg)
// 				}
// 				if _, err := c.Handler.Send(c.Conn, msg.Buffer); err != nil {
// 					c.Conn.Close()
// 				}
// 				c.Handler.OnMessageDone(c, msg)
// 			} else {
// 				c.dropMessage(msg)
// 			}
// 		case <-c.chClose:
// 			// clear msg in send queue
// 			for {
// 				select {
// 				case msg := <-c.chSend:
// 					c.Handler.OnMessageDone(c, msg)
// 				default:
// 					return
// 				}
// 			}
// 		}
// 	}
// }

// func (c *Client) batchSendLoop() {
// 	var msg *Message
// 	var chLen int
// 	var coders []MessageCoder
// 	var buffer = c.Handler.Malloc(2048)[0:0]
// 	var sendBufferSize = c.Handler.SendBufferSize()
// 	defer c.Handler.Free(buffer)

// 	for {
// 		select {
// 		case msg = <-c.chSend:
// 		case <-c.chClose:
// 			// clear msg in send queue
// 			for {
// 				select {
// 				case msg := <-c.chSend:
// 					c.Handler.OnMessageDone(c, msg)
// 				default:
// 					return
// 				}
// 			}
// 		}
// 		if !c.reconnecting {
// 			chLen = len(c.chSend)
// 			coders = c.Handler.Coders()
// 			for i := 0; i < chLen && len(buffer) < sendBufferSize; i++ {
// 				if len(buffer) == 0 {
// 					for j := 0; j < len(coders); j++ {
// 						msg = coders[j].Encode(c, msg)
// 					}
// 					buffer = c.Handler.Append(buffer, msg.Buffer...)
// 					c.Handler.OnMessageDone(c, msg)
// 				}
// 				msg = <-c.chSend
// 				for j := 0; j < len(coders); j++ {
// 					msg = coders[j].Encode(c, msg)
// 				}
// 				buffer = c.Handler.Append(buffer, msg.Buffer...)
// 				c.Handler.OnMessageDone(c, msg)
// 			}
// 			if len(buffer) == 0 {
// 				for j := 0; j < len(coders); j++ {
// 					msg = coders[j].Encode(c, msg)
// 				}
// 				_, err := c.Handler.Send(c.Conn, msg.Buffer)
// 				if err != nil {
// 					c.Conn.Close()
// 				}
// 				c.Handler.OnMessageDone(c, msg)
// 			} else {
// 				if _, err := c.Handler.Send(c.Conn, buffer); err != nil {
// 					c.Conn.Close()
// 				}
// 				buffer = buffer[0:0]
// 			}
// 		} else {
// 			c.dropMessage(msg)
// 		}
// 	}
// }

func newClientWithConn(conn net.Conn, codec codec.Codec, handler Handler, onStop func(*Client)) *Client {
	log.Info("%v\t%v\tConnected", handler.LogTag(), conn.RemoteAddr())

	c := &Client{}
	c.Conn = conn
	c.Writer = bufio.NewWriterSize(conn, handler.SendBufferSize())
	c.Codec = codec
	c.Handler = handler
	c.Head = make([]byte, 4)
	// c.chSend = make(chan *Message, c.Handler.SendQueueSize())
	c.chClose = make(chan util.Empty)
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]*asyncHandler)
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
	c.Writer = bufio.NewWriterSize(conn, handler.SendBufferSize())
	c.Codec = codec.DefaultCodec
	c.Handler = handler
	c.Dialer = dialer
	c.Head = make([]byte, 4)
	// c.chSend = make(chan *Message, c.Handler.SendQueueSize())
	c.chClose = make(chan util.Empty)
	c.sessionMap = make(map[uint64]*rpcSession)
	c.asyncHandlerMap = make(map[uint64]*asyncHandler)

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

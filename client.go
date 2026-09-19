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
	"unsafe"

	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

const (
	// TimeZero as a push timeout means not to wait if the send queue is full.
	TimeZero time.Duration = 0
	// TimeForever as a push timeout means to wait until the message is queued
	// or the Client stops.
	TimeForever time.Duration = 1<<63 - 1
)

// ReconnectInfo describes a single reconnect attempt of a client-role Client.
// It is passed to Handler.OnReconnect after every attempt.
type ReconnectInfo struct {
	// Times is the 1-based attempt number in the current reconnect round;
	// it starts over from 1 each time the connection is lost.
	Times int
	// MaxTimes is Handler.MaxReconnectTimes(); <= 0 means unlimited.
	MaxTimes int
	// Addr is the remote address of the lost connection.
	Addr string
	// Success reports whether the Dial succeeded.
	Success bool
	// Err is the Dial error, nil on success.
	Err error
}

// DialerFunc dials the server. A client-role Client also calls it to
// reconnect and restart.
type DialerFunc func() (net.Conn, error)

// rpcSession is a pending blocking call waiting for its response on done.
// done is closed without a response if the call is dropped or the conn breaks.
type rpcSession struct {
	seq  uint64
	done chan *Message
}

// newSession creates an rpcSession for seq.
func newSession(seq uint64) *rpcSession {
	return &rpcSession{seq: seq, done: make(chan *Message, 1)}
}

// Client is one arpc connection, on either side: a client-role Client dials
// the server (see NewClient) and reconnects when the conn breaks, while a
// server-role Client is created by a Server for each accepted conn. Both can
// make calls to and serve calls from the peer.
//
// A Client is safe for concurrent use; there may be multiple outstanding
// calls at a time.
type Client struct {
	// seq is kept 64-bit aligned on 32-bit platforms.
	seq uint64

	// Conn is the current conn; it changes on reconnect and Restart.
	Conn net.Conn
	// Codec encodes and decodes message bodies.
	Codec codec.Codec
	// Handler handles the Client's messages and events.
	Handler Handler
	// Reader reads from Conn, wrapped by Handler.WrapReader with BatchRecv.
	Reader io.Reader
	// Dialer dials the server; nil for a server-role Client.
	Dialer DialerFunc
	// Head is the buffer for reading the body length of each message.
	Head Header

	running      bool
	reconnecting bool

	mux             sync.Mutex
	sessionMap      map[uint64]*rpcSession
	asyncHandlerMap map[uint64]*asyncHandler
	streamLocalMap  map[uint64]*Stream
	streamRemoteMap map[uint64]*Stream

	chSend  chan *Message
	chClose chan util.Empty
	// recvDone is closed when the current generation's recvLoop returns.
	// Restart waits on it so that the old loop's teardown never overlaps with,
	// and clobbers, the state of the new generation.
	recvDone chan struct{}

	// The writev send queue, used with Handler.AsyncWritev. writevBuffers
	// holds the buffers to write and writevMsgs the queued Messages in order,
	// for the callbacks after writing. writevSending reports whether a
	// writevLoop is running, so that there is at most one per Client.
	writevMux     sync.Mutex
	writevBuffers net.Buffers
	writevMsgs    []*Message
	writevSending bool

	onStop func(*Client)

	// sfGroup de-duplicates concurrent calls of methods enabled by
	// Handler.Singleflight.
	sfGroup singleflightGroup

	values map[interface{}]interface{}
	// UserData interface{}
}

// SetState sets the running state directly. It is only meant for Clients
// driven by an external non-blocking conn framework.
func (c *Client) SetState(running bool) {
	c.running = running
}

// IsClient reports whether c is a client-role Client, i.e. it has a Dialer.
func (c *Client) IsClient() bool {
	return c.Dialer != nil
}

// IsServer reports whether c is a server-role Client, i.e. it has no Dialer.
func (c *Client) IsServer() bool {
	return c.Dialer == nil
}

// Get returns the value stored on the Client for key. Values are local and
// cleared by Restart.
func (c *Client) Get(key interface{}) (interface{}, bool) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if len(c.values) == 0 {
		return nil, false
	}
	value, ok := c.values[key]
	return value, ok
}

// Set stores a key-value pair on the Client. It does nothing if key or value
// is nil.
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

// Delete deletes the value for key.
func (c *Client) Delete(key interface{}) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.values != nil {
		delete(c.values, key)
	}
}

// Ping writes a ping message directly to the conn, bypassing the send queue.
func (c *Client) Ping() {
	c.Conn.Write(PingMessage.Buffer)
}

// Pong writes a pong message directly to the conn, bypassing the send queue.
func (c *Client) Pong() {
	c.Conn.Write(PongMessage.Buffer)
}

// Keepalive sends a Ping every interval (30s if interval <= 0) while the
// Client is running. It returns immediately.
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

// NewMessage creates a Message with a new sequence number and the Client's
// Handler and Codec. args[0], if any, must be a map[interface{}]interface{} of
// message values.
func (c *Client) NewMessage(cmd byte, method string, v interface{}, args ...interface{}) *Message {
	if len(args) == 0 {
		return newMessage(cmd, method, v, false, false, atomic.AddUint64(&c.seq, 1), c.Handler, c.Codec, nil)
	}
	return newMessage(cmd, method, v, false, false, atomic.AddUint64(&c.seq, 1), c.Handler, c.Codec, args[0].(map[interface{}]interface{}))
}

// Call sends a request for method with req and waits for the response,
// decoding it into rsp. timeout must be > 0 and covers both queuing and
// waiting. args[0], if any, must be a map[interface{}]interface{} of message
// values.
//
// req and rsp may be []byte, string or pointers to them, or values handled
// by the Client's Codec; see util.ValueToBytes. It returns ErrClientTimeout
// on timeout, or the remote error for an error response.
func (c *Client) Call(method string, req interface{}, rsp interface{}, timeout time.Duration, args ...interface{}) error {
	if err := c.checkCallArgs(method, timeout); err != nil {
		return err
	}

	// Turn the timeout into a context to share the path of CallContext.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if key, ok := c.Handler.SingleflightKey(method, req); ok {
		return c.callSingleflight(ctx, method, req, rsp, key, args...)
	}

	return c.callContext(ctx, method, req, rsp, args...)
}

// callContext is the body of Call and CallContext without singleflight: it
// sends the request and decodes the response into rsp.
func (c *Client) callContext(ctx context.Context, method string, req interface{}, rsp interface{}, args ...interface{}) error {
	msg := c.newRequestMessage(CmdRequest, method, req, false, false, args...)
	resp, err := c.sendRequest(ctx, msg)
	if err != nil {
		return err
	}
	err = c.parseResponse(resp, rsp)
	c.Handler.OnMessageDone(c, resp)
	return err
}

// callSingleflight is Call/CallContext for a method with singleflight
// enabled. Concurrent calls with the same key send only one request: the
// leader sends it, and followers wait for its response and each decode it into
// their own rsp. Each caller still honors its own ctx.
func (c *Client) callSingleflight(ctx context.Context, method string, req interface{}, rsp interface{}, key string, args ...interface{}) error {
	k := sfKey{method: method, key: key}
	call, leader := c.sfGroup.acquire(k)
	if leader {
		data, err := c.requestData(ctx, method, req, args...)
		c.sfGroup.finish(k, call, data, err)
		if err != nil {
			return err
		}
		return c.parseData(data, rsp)
	}

	select {
	case <-call.done:
	case <-ctx.Done():
		return ErrClientTimeout
	case <-c.chClose:
		return ErrClientStopped
	}
	if call.err != nil {
		return call.err
	}
	return c.parseData(call.data, rsp)
}

// writeSync encodes msg with the coders and writes it on the calling
// goroutine, used when neither AsyncWritev nor AsyncWrite is enabled. The
// conn is closed on a write error. While reconnecting, msg is dropped and
// ErrClientReconnecting is returned.
func (c *Client) writeSync(msg *Message) error {
	if c.reconnecting {
		c.dropMessage(msg)
		return ErrClientReconnecting
	}
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
}

// sendRequest sends the request msg and waits for its response, or returns
// ErrClientTimeout when ctx is done. The caller must pass the returned
// response to OnMessageDone after decoding it. The response is nil if the
// call was dropped or the conn broke.
func (c *Client) sendRequest(ctx context.Context, msg *Message) (*Message, error) {
	seq := msg.Seq()
	sess := newSession(seq)
	c.addSession(seq, sess)
	defer c.deleteSession(seq)

	if c.Handler.AsyncWritev() {
		if err := c.pushWritev(msg); err != nil {
			return nil, err
		}
	} else if c.Handler.AsyncWrite() {
		select {
		case c.chSend <- msg:
		case <-ctx.Done():
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return nil, ErrClientTimeout
		case <-c.chClose:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return nil, ErrClientStopped
		}
	} else {
		if err := c.writeSync(msg); err != nil {
			return nil, err
		}
	}

	select {
	case resp := <-sess.done:
		return resp, nil
	case <-ctx.Done():
		return nil, ErrClientTimeout
	case <-c.chClose:
		return nil, ErrClientStopped
	}
}

// requestData is the request of a singleflight leader: it sends the request
// and returns a copy of the response data, which stays valid after the
// response is released and can be decoded with parseData.
func (c *Client) requestData(ctx context.Context, method string, req interface{}, args ...interface{}) ([]byte, error) {
	msg := c.newRequestMessage(CmdRequest, method, req, false, false, args...)
	resp, err := c.sendRequest(ctx, msg)
	if err != nil {
		return nil, err
	}
	data, err := c.responseData(resp)
	c.Handler.OnMessageDone(c, resp)
	return data, err
}

// responseData returns a copy of the response data, or the remote error for
// an error response.
func (c *Client) responseData(msg *Message) ([]byte, error) {
	if msg == nil {
		return nil, ErrClientReconnecting
	}
	switch msg.Cmd() {
	case CmdResponse:
		if msg.IsError() {
			return nil, msg.Error()
		}
		return append([]byte{}, msg.Data()...), nil
	default:
		return nil, ErrInvalidRspMessage
	}
}

// parseData decodes data returned by requestData into rsp, the same way as
// parseResponse.
func (c *Client) parseData(data []byte, rsp interface{}) error {
	if rsp == nil {
		return nil
	}
	switch vt := rsp.(type) {
	case *string:
		*vt = string(data)
	case *[]byte:
		*vt = append([]byte{}, data...)
	default:
		return c.Codec.Unmarshal(data, rsp)
	}
	return nil
}

// CallWith is an alias of CallContext.
func (c *Client) CallWith(ctx context.Context, method string, req interface{}, rsp interface{}, args ...interface{}) error {
	return c.CallContext(ctx, method, req, rsp, args...)
}

// CallContext is like Call, but waits until ctx is done instead of a timeout.
func (c *Client) CallContext(ctx context.Context, method string, req interface{}, rsp interface{}, args ...interface{}) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}

	if key, ok := c.Handler.SingleflightKey(method, req); ok {
		return c.callSingleflight(ctx, method, req, rsp, key, args...)
	}

	return c.callContext(ctx, method, req, rsp, args...)
}

// CallAsync sends a request for method with req and returns without waiting
// for the response. handler must not be nil and timeout must be > 0. handler
// is called at most once: with the response, with ErrTimeout if none arrives
// within timeout, or with ErrClientReconnecting if the conn breaks. It is not
// called if CallAsync returns an error or the request is dropped unsent.
func (c *Client) CallAsync(method string, req interface{}, handler AsyncHandlerFunc, timeout time.Duration, args ...interface{}) error {
	err := c.checkCallAsyncArgs(method, handler, timeout)
	if err != nil {
		return err
	}

	if key, ok := c.Handler.SingleflightKey(method, req); ok {
		return c.callAsyncSingleflight(method, req, handler, timeout, key, args...)
	}

	return c.callAsyncOnce(method, req, handler, timeout, args...)
}

// callAsyncSingleflight is CallAsync for a method with singleflight enabled.
// Concurrent calls with the same key send only one request: the leader sends
// it, and followers' handlers are called with its response, or with
// ErrTimeout if their own timeout comes first.
func (c *Client) callAsyncSingleflight(method string, req interface{}, handler AsyncHandlerFunc, timeout time.Duration, key string, args ...interface{}) error {
	k := sfKey{method: method, key: key, async: true}
	call, leader := c.sfGroup.acquire(k)

	if leader {
		// Call the leader's handler, then the followers', then release the call.
		internal := func(ctx *Context, err error) {
			handler(ctx, err)
			call.fanout(ctx, err)
			c.sfGroup.release(k, call)
		}
		if err := c.callAsyncOnce(method, req, internal, timeout, args...); err != nil {
			// The request was not sent: pass the error to the followers. The
			// leader gets it as the returned error, and its handler is not
			// called, as with a plain CallAsync.
			call.fanout(nil, err)
			c.sfGroup.release(k, call)
			return err
		}
		return nil
	}

	// Follower: subscribe to the leader's result, with its own timeout.
	sub := &sfAsyncSub{handler: handler}
	sub.timer = time.AfterFunc(timeout, func() {
		sub.fire(nil, ErrTimeout)
	})
	if !call.addSub(sub) {
		// The leader has finished; send a request of its own.
		sub.timer.Stop()
		return c.callAsyncOnce(method, req, handler, timeout, args...)
	}
	return nil
}

// callAsyncOnce sends the request of a CallAsync and registers its handler,
// with a timer that calls the handler with ErrTimeout. With AsyncWrite, the
// same timeout also bounds waiting for the send queue.
func (c *Client) callAsyncOnce(method string, req interface{}, handler AsyncHandlerFunc, timeout time.Duration, args ...interface{}) error {
	var err error

	msg := c.newRequestMessage(CmdRequest, method, req, false, true, args...)
	seq := msg.Seq()

	chTimer := make(chan time.Time, 1)
	timerC := *(*<-chan time.Time)(unsafe.Pointer(&chTimer))
	timer := &time.Timer{C: timerC}
	timerCallback := time.AfterFunc(timeout, func() {
		ah, ok := c.getAndDeleteAsyncHandler(seq)
		if ok {
			if ah.timer != nil {
				ah.timer.Stop()
			}
			ah.handler(nil, ErrTimeout)
			putAsyncHandler(ah)
		}
		chTimer <- time.Now()
	})
	ah := getAsyncHandler(timerCallback, handler)
	c.addAsyncHandler(seq, ah)

	if c.Handler.AsyncWritev() {
		err = c.pushWritev(msg)
	} else if c.Handler.AsyncWrite() {
		err = c.pushMessage(msg, timer)
	} else {
		err = c.writeSync(msg)
	}

	if err != nil && handler != nil {
		c.deleteAsyncHandler(seq)
		timerCallback.Stop()
	}

	return err
}

// Notify sends a notify for method with data; the peer sends no response.
// With AsyncWrite, timeout bounds waiting for the send queue, and 0 means not
// to wait (ErrClientOverstock if full); it must not be negative. args[0], if
// any, must be a map[interface{}]interface{} of message values.
func (c *Client) Notify(method string, data interface{}, timeout time.Duration, args ...interface{}) error {
	err := c.checkNotifyArgs(method, timeout)
	if err != nil {
		return err
	}

	msg := c.newRequestMessage(CmdNotify, method, data, false, true, args...)

	if c.Handler.AsyncWritev() {
		err = c.pushWritev(msg)
	} else if c.Handler.AsyncWrite() {
		switch timeout {
		case TimeZero:
			err = c.pushMessage(msg, nil)
		default:
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			err = c.pushMessage(msg, timer)
		}
	} else {
		err = c.writeSync(msg)
	}

	return err
}

// NotifyWith is an alias of NotifyContext.
func (c *Client) NotifyWith(ctx context.Context, method string, data interface{}, args ...interface{}) error {
	return c.NotifyContext(ctx, method, data, args...)
}

// NotifyContext is like Notify, but waits for the send queue until ctx is
// done instead of a timeout.
func (c *Client) NotifyContext(ctx context.Context, method string, data interface{}, args ...interface{}) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}

	msg := c.newRequestMessage(CmdNotify, method, data, false, true, args...)

	if c.Handler.AsyncWritev() {
		return c.pushWritev(msg)
	} else if c.Handler.AsyncWrite() {
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
		return c.writeSync(msg)
	}

	return nil
}

// PushMsg sends msg as is. With AsyncWrite, timeout bounds waiting for the
// send queue: TimeZero means not to wait (ErrClientOverstock if full), and
// TimeForever or a negative value means to wait until queued or stopped.
// msg is passed to OnMessageDone or OnOverstock if it cannot be sent.
func (c *Client) PushMsg(msg *Message, timeout time.Duration) error {
	err := c.CheckState()
	if err != nil {
		c.Handler.OnMessageDone(c, msg)
		return err
	}

	if c.Handler.AsyncWritev() {
		return c.pushWritev(msg)
	}

	if !c.Handler.AsyncWrite() {
		return c.writeSync(msg)
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

// Restart stops the Client, dials a new conn, and starts it again with fresh
// state: pending calls, Streams and values are dropped. It returns the
// Dialer's error if dialing fails. It is only for client-role Clients.
func (c *Client) Restart() error {
	c.mux.Lock()
	recvDone := c.recvDone
	c.mux.Unlock()

	c.Stop()

	// Wait for the old recvLoop to exit before building the new generation.
	// Otherwise its teardown (reconnect cleanup and closeAndClean) may run
	// after the new state is installed, e.g. setting running back to false.
	// Stop has already closed the conn and chClose, so it returns promptly.
	if recvDone != nil {
		<-recvDone
	}

	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		conn, err := c.Dialer()
		if err != nil {
			return err
		}

		preConn := c.Conn
		c.Conn = conn

		c.chClose = make(chan util.Empty)
		c.sessionMap = make(map[uint64]*rpcSession)
		c.asyncHandlerMap = make(map[uint64]*asyncHandler)
		c.streamLocalMap = make(map[uint64]*Stream)
		c.streamRemoteMap = make(map[uint64]*Stream)
		c.values = map[interface{}]interface{}{}

		c.writevMux.Lock()
		c.writevBuffers = nil
		c.writevMsgs = nil
		c.writevSending = false
		c.writevMux.Unlock()

		c.initReader()
		// AsyncWritev takes precedence: it uses no chSend or sendLoop.
		if c.Handler.AsyncWrite() && !c.Handler.AsyncWritev() {
			c.chSend = make(chan *Message, c.Handler.SendQueueSize())
		}
		// The new loops watch this generation's channels only.
		chSend, chClose := c.chSend, c.chClose
		recvDone := make(chan struct{})
		c.recvDone = recvDone
		if c.Handler.AsyncWrite() && !c.Handler.AsyncWritev() {
			go util.Safe(func() { c.sendLoop(chSend, chClose) })
		}
		go util.Safe(func() {
			defer close(recvDone)
			c.recvLoop(chClose)
		})

		c.running = true
		c.reconnecting = false

		log.Info("%v\t[%v] Restarted to [%v]", c.Handler.LogTag(), preConn.RemoteAddr(), conn.RemoteAddr())
	}

	return nil
}

// chanClosed reports whether ch has been closed. ch is a close-only signal
// channel (never sent to), so a ready receive means it is closed.
func chanClosed(ch chan util.Empty) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// closeChan closes ch if it is not closed yet. The caller must hold c.mux so
// that the check-and-close is atomic between Stop and closeAndClean of the
// same generation.
func closeChan(ch chan util.Empty) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// Stop stops the Client and closes its conn; a client-role Client does not
// reconnect after it. OnDisconnected is called by the read goroutine once it
// exits.
func (c *Client) Stop() {
	c.mux.Lock()
	c.running = false
	// Signal the current generation's loops to stop. Restart installs a new
	// chClose for the next generation, so the old loops keep seeing their own
	// closed channel and never resume even after running is set to true again.
	closeChan(c.chClose)
	c.mux.Unlock()

	c.Conn.Close()
}

// closeAndClean stops the generation owning chClose: it closes chClose (see
// closeChan) and the conn, drops the queued writev messages, and calls onStop
// and OnDisconnected.
func (c *Client) closeAndClean(chClose chan util.Empty) {
	c.mux.Lock()
	c.running = false
	closeChan(chClose)
	c.mux.Unlock()

	c.Conn.Close()

	// Drop the messages still in the writev queue. A batch belongs to whoever
	// takes it out under writevMux, so this never handles the messages a
	// running writevLoop has taken.
	c.drainWritev()

	if c.onStop != nil {
		c.onStop(c)
	}

	c.Handler.OnDisconnected(c)
}

// drainWritev drops all messages in the writev queue.
func (c *Client) drainWritev() {
	c.writevMux.Lock()
	msgs := c.writevMsgs
	c.writevBuffers = nil
	c.writevMsgs = nil
	c.writevMux.Unlock()
	for _, m := range msgs {
		c.dropMessage(m)
	}
}

// CheckState returns ErrClientStopped if the Client is not running,
// ErrClientReconnecting if it is reconnecting, or nil.
func (c *Client) CheckState() error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}
	return nil
}

// checkCallArgs checks the state, method and timeout (must be > 0) of Call.
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

// checkCallAsyncArgs checks the state, method, handler and timeout (must be
// > 0) of CallAsync.
func (c *Client) checkCallAsyncArgs(method string, handler AsyncHandlerFunc, timeout time.Duration) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}
	if timeout == 0 {
		return ErrClientInvalidTimeoutZero
	}
	if timeout < 0 {
		return ErrClientInvalidTimeoutLessThanZero
	}
	if handler == nil {
		return ErrClientInvalidAsyncHandler
	}
	return nil
}

// checkNotifyArgs checks the state, method and timeout (must be >= 0) of
// Notify.
func (c *Client) checkNotifyArgs(method string, timeout time.Duration) error {
	if err := c.checkStateAndMethod(method); err != nil {
		return err
	}
	if timeout < 0 {
		return ErrClientInvalidTimeoutLessThanZero
	}
	return nil
}

// checkStateAndMethod checks the Client's state and the method name.
func (c *Client) checkStateAndMethod(method string) error {
	err := c.CheckState()
	if err != nil {
		return err
	}
	return checkMethod(method)
}

// pushMessage pushes msg into chSend. With a nil timer it does not wait and
// returns ErrClientOverstock if the queue is full; otherwise it waits until
// the timer fires and returns ErrClientTimeout.
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

// pushWritev appends msg to the writev queue and starts a writevLoop if none
// is running. It never blocks on I/O.
func (c *Client) pushWritev(msg *Message) error {
	if c.reconnecting {
		c.dropMessage(msg)
		return ErrClientReconnecting
	}

	// Coders work on a contiguous Buffer and may return a new Message, so run
	// them before queuing. newRequestMessage builds contiguous messages when
	// there are coders.
	if coders := c.Handler.Coders(); len(coders) > 0 {
		for j := 0; j < len(coders); j++ {
			msg = coders[j].Encode(c, msg)
		}
	}

	c.writevMux.Lock()
	if !c.running {
		c.writevMux.Unlock()
		c.Handler.OnMessageDone(c, msg)
		return ErrClientStopped
	}
	c.writevBuffers = append(c.writevBuffers, msg.Buffer)
	if len(msg.body) > 0 {
		c.writevBuffers = append(c.writevBuffers, msg.body)
	}
	c.writevMsgs = append(c.writevMsgs, msg)
	launch := !c.writevSending
	if launch {
		c.writevSending = true
	}
	c.writevMux.Unlock()

	if launch {
		go util.Safe(c.writevLoop)
	}
	return nil
}

// writevLoop writes the writev queue batch by batch, each with one writev,
// and exits when the queue is empty. It is started by pushWritev, and at most
// one runs per Client.
func (c *Client) writevLoop() {
	closed := false
	for {
		c.writevMux.Lock()
		if len(c.writevBuffers) == 0 {
			c.writevSending = false
			c.writevMux.Unlock()
			return
		}
		bufs := c.writevBuffers
		msgs := c.writevMsgs
		c.writevBuffers = nil
		c.writevMsgs = nil
		c.writevMux.Unlock()

		// Once the conn is broken, drop the queued messages instead of writing
		// them, but keep looping until the queue is empty so that writevSending
		// is only cleared then.
		if closed || c.reconnecting {
			for _, m := range msgs {
				c.dropMessage(m)
			}
			closed = true
			continue
		}

		// SendN consumes bufs, so the callbacks below go by msgs.
		if _, err := c.Handler.SendN(c.Conn, bufs); err != nil {
			c.Conn.Close()
			for _, m := range msgs {
				c.Handler.OnMessageDone(c, m)
			}
			closed = true
			continue
		}

		for _, m := range msgs {
			c.Handler.OnMessageDone(c, m)
		}
	}
}

// newRequestMessage creates an outgoing message with a new sequence number.
// args[0], if any, must be a map[interface{}]interface{} of message values.
func (c *Client) newRequestMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, args ...interface{}) *Message {
	var values map[interface{}]interface{}
	if len(args) > 0 {
		values = args[0].(map[interface{}]interface{})
	}
	seq := atomic.AddUint64(&c.seq, 1)
	// Keep the data apart from the header only for writev without coders, as
	// coders need a contiguous buffer.
	if c.Handler.AsyncWritev() && len(c.Handler.Coders()) == 0 {
		return newWritevMessage(cmd, method, v, isError, isAsync, seq, c.Handler, c.Codec, values)
	}
	return newMessage(cmd, method, v, isError, isAsync, seq, c.Handler, c.Codec, values)
}

// parseResponse decodes the response msg into rsp: *string and *[]byte get a
// copy of the data, other types are decoded by the Codec. It returns the
// remote error for an error response, and ErrClientReconnecting for a nil msg.
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

// addSession registers a pending call, unless the Client is stopped.
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

// clearSession fails all pending calls by closing their done channels.
func (c *Client) clearSession() {
	c.mux.Lock()
	for _, sess := range c.sessionMap {
		close(sess.done)
	}
	c.sessionMap = make(map[uint64]*rpcSession)
	c.mux.Unlock()
}

// dropMessage gives up an unsent message: it fails its pending call, if any,
// and calls OnMessageDropped.
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

// addAsyncHandler registers a pending CallAsync, unless the Client is stopped.
func (c *Client) addAsyncHandler(seq uint64, ah *asyncHandler) {
	c.mux.Lock()
	if c.running {
		c.asyncHandlerMap[seq] = ah
	}
	c.mux.Unlock()
}

func (c *Client) deleteAsyncHandler(seq uint64) {
	c.mux.Lock()
	ah, ok := c.asyncHandlerMap[seq]
	if ok {
		delete(c.asyncHandlerMap, seq)
		putAsyncHandler(ah)
	}
	c.mux.Unlock()
}

func (c *Client) getAndDeleteAsyncHandler(seq uint64) (*asyncHandler, bool) {
	c.mux.Lock()
	ah, ok := c.asyncHandlerMap[seq]
	if ok {
		delete(c.asyncHandlerMap, seq)
		c.mux.Unlock()
	} else {
		c.mux.Unlock()
	}

	return ah, ok
}

// clearAsyncHandler calls all pending CallAsync handlers with
// ErrClientReconnecting, via AsyncExecute.
func (c *Client) clearAsyncHandler() {
	c.mux.Lock()
	handlers := c.asyncHandlerMap
	c.asyncHandlerMap = make(map[uint64]*asyncHandler)
	c.mux.Unlock()
	for _, ah := range handlers {
		if ah.timer != nil {
			ah.timer.Stop()
		}
		c.Handler.AsyncExecute(func() {
			ah.handler(nil, ErrClientReconnecting)
			putAsyncHandler(ah)
		})
	}
}

func (c *Client) deleteStream(id uint64, local bool) {
	c.mux.Lock()
	if c.running {
		var streamMap map[uint64]*Stream
		if local {
			streamMap = c.streamLocalMap
		} else {
			streamMap = c.streamRemoteMap
		}
		delete(streamMap, id)
	}
	c.mux.Unlock()
}

// getStreamAndPushMsg returns the Stream for id; with done, it also removes
// the Stream from the Client. local selects the Streams created on this side.
func (c *Client) getStreamAndPushMsg(id uint64, local, done bool) (stream *Stream, ok bool) {
	c.mux.Lock()
	if c.running {
		var streamMap map[uint64]*Stream
		if local {
			streamMap = c.streamLocalMap
		} else {
			streamMap = c.streamRemoteMap
		}
		stream, ok = streamMap[id]
		if ok && done {
			delete(streamMap, id)
		}
	}
	c.mux.Unlock()
	return stream, ok
}

// clearStream closes and removes all Streams.
func (c *Client) clearStream() {
	c.mux.Lock()
	streamLocalMap := c.streamLocalMap
	streamRemoteMap := c.streamRemoteMap
	c.streamLocalMap = make(map[uint64]*Stream)
	c.streamRemoteMap = make(map[uint64]*Stream)
	c.mux.Unlock()
	for _, stream := range streamLocalMap {
		stream.CloseSend()
		stream.CloseRecv()
	}
	for _, stream := range streamRemoteMap {
		stream.CloseSend()
		stream.CloseRecv()
	}
}

// run starts the read loop, and the send loop with AsyncWrite, if the Client
// is not running.
func (c *Client) run() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.initReader()
		// Capture this generation's channels; a later Restart installs new ones.
		chSend, chClose := c.chSend, c.chClose
		recvDone := make(chan struct{})
		c.recvDone = recvDone
		// AsyncWritev takes precedence: pushWritev starts a writer on demand,
		// so there is no sendLoop.
		if c.Handler.AsyncWrite() && !c.Handler.AsyncWritev() {
			go util.Safe(func() { c.sendLoop(chSend, chClose) })
		}
		go util.Safe(func() {
			defer close(recvDone)
			c.recvLoop(chClose)
		})
	}
}

// initReader sets Reader for the current conn according to BatchRecv.
func (c *Client) initReader() {
	if c.Handler.BatchRecv() {
		c.Reader = c.Handler.WrapReader(c.Conn)
	} else {
		c.Reader = c.Conn
	}
}

// recvLoop reads and dispatches messages for one generation. When the conn
// breaks, a server-role Client stops, while a client-role Client fails its
// pending calls and Streams and reconnects, stopping once the attempts run
// out. The loop exits once chClose is closed; checking its own chClose rather
// than c.running keeps it from resuming after a Restart sets running again.
func (c *Client) recvLoop(chClose chan util.Empty) {
	var (
		err  error
		msg  *Message
		addr = c.Conn.RemoteAddr().String()
	)

	log.Debug("%v\t%v\trecvLoop start", c.Handler.LogTag(), addr)
	defer log.Debug("%v\t%v\trecvLoop stop", c.Handler.LogTag(), addr)

	// Every exit stops this generation, including when chClose was closed
	// before the loop started or while a message was being handled, so that
	// onStop and OnDisconnected are always called once.
	defer c.closeAndClean(chClose)

	if c.Dialer == nil {
		for !chanClosed(chClose) {
			msg, err = c.Handler.Recv(c)
			if err != nil {
				log.Error("%v\t%v\tDisconnected: %v", c.Handler.LogTag(), addr, err)
				return
			}
			c.Handler.OnMessage(c, msg)
		}
	} else {
		go c.Handler.OnConnected(c)

		for !chanClosed(chClose) {
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
			c.clearStream()

			// if c.running {
			// 	log.Info("%v\t%v\tReconnect Start", c.Handler.LogTag(), addr)
			// }
			maxReconnectTimes := c.Handler.MaxReconnectTimes()
			for i := 0; !chanClosed(chClose) && ((maxReconnectTimes <= 0) || (i < maxReconnectTimes)); i++ {
				log.Info("%v\t%v\tReconnect Trying %v", c.Handler.LogTag(), addr, i)
				conn, err := c.Dialer()
				info := &ReconnectInfo{
					Times:    i + 1,
					MaxTimes: maxReconnectTimes,
					Addr:     addr,
					Success:  err == nil,
					Err:      err,
				}
				if err == nil {
					c.Conn = conn

					c.initReader()

					c.reconnecting = false

					log.Info("%v\t%v\tReconnected", c.Handler.LogTag(), addr)

					// Called synchronously before OnConnected; c.Conn is
					// already the new conn here.
					c.Handler.OnReconnect(c, info)

					go c.Handler.OnConnected(c)

					goto RECV
				}

				log.Info("%v\t%v\tReconnect Failed %v: %v", c.Handler.LogTag(), addr, i, err)
				c.Handler.OnReconnect(c, info)

				time.Sleep(time.Second)
			}
			// Stopped, or out of reconnect attempts.
			return
		}
	}
}

// sendLoop sends messages from chSend for one generation until chClose is
// closed, then drains the remaining ones. Working on its own channels keeps a
// superseded loop from competing with the one a Restart starts.
func (c *Client) sendLoop(chSend chan *Message, chClose chan util.Empty) {
	addr := c.Conn.RemoteAddr().String()
	log.Debug("%v\t%v\tsendLoop start", c.Handler.LogTag(), addr)
	defer log.Debug("%v\t%v\tsendLoop stop", c.Handler.LogTag(), addr)

	if c.Handler.BatchSend() {
		c.batchSendLoop(chSend, chClose)
	} else {
		c.normalSendLoop(chSend, chClose)
	}
}

// normalSendLoop writes queued messages one by one. While reconnecting, they
// are dropped.
func (c *Client) normalSendLoop(chSend chan *Message, chClose chan util.Empty) {
	var msg *Message
	var coders []MessageCoder
	for {
		select {
		case msg = <-chSend:
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
		case <-chClose:
			// clear msg in send queue
			for {
				select {
				case msg := <-chSend:
					c.Handler.OnMessageDone(c, msg)
				default:
					return
				}
			}
		}
	}
}

// batchSendLoop is like normalSendLoop, but merges the messages already
// queued into one write while the buffer is under SendBufferSize.
func (c *Client) batchSendLoop(chSend chan *Message, chClose chan util.Empty) {
	var msg *Message
	var chLen int
	var coders []MessageCoder
	var buffer = c.Handler.Malloc(2048)[0:0]
	var sendBufferSize = c.Handler.SendBufferSize()
	// Free the buffer held at exit: Append may have replaced, and freed, the
	// one allocated above.
	defer func() { c.Handler.Free(buffer) }()

	for {
		select {
		case msg = <-chSend:
		case <-chClose:
			// clear msg in send queue
			for {
				select {
				case msg := <-chSend:
					c.Handler.OnMessageDone(c, msg)
				default:
					return
				}
			}
		}
		if !c.reconnecting {
			chLen = len(chSend)
			coders = c.Handler.Coders()
			for i := 0; i < chLen && len(buffer) < sendBufferSize; i++ {
				if len(buffer) == 0 {
					for j := 0; j < len(coders); j++ {
						msg = coders[j].Encode(c, msg)
					}
					buffer = c.Handler.Append(buffer, msg.Buffer...)
					c.Handler.OnMessageDone(c, msg)
				}
				msg = <-chSend
				for j := 0; j < len(coders); j++ {
					msg = coders[j].Encode(c, msg)
				}
				buffer = c.Handler.Append(buffer, msg.Buffer...)
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

// newClientWithConn creates and starts a server-role Client for an accepted
// conn. onStop is called when it stops.
func newClientWithConn(conn net.Conn, codec codec.Codec, handler Handler, onStop func(*Client)) *Client {
	log.Info("%v\t%v\tConnected", handler.LogTag(), conn.RemoteAddr())

	c := &Client{
		seq:             1,
		Conn:            conn,
		Codec:           codec,
		Handler:         handler,
		Head:            make([]byte, 4),
		chClose:         make(chan util.Empty),
		sessionMap:      make(map[uint64]*rpcSession),
		asyncHandlerMap: make(map[uint64]*asyncHandler),
		streamLocalMap:  make(map[uint64]*Stream),
		streamRemoteMap: make(map[uint64]*Stream),
		onStop:          onStop,
	}
	if c.Handler.AsyncWrite() && !c.Handler.AsyncWritev() {
		c.chSend = make(chan *Message, handler.SendQueueSize())
	}

	c.run()

	return c
}

// NewClient dials with dialer and starts a client-role Client on the conn.
// args[0], if it is a Handler, is used as is; otherwise the Client uses a
// clone of DefaultHandler. The Codec is codec.DefaultCodec.
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

	c := &Client{
		seq:             1,
		Conn:            conn,
		Codec:           codec.DefaultCodec,
		Handler:         handler,
		Dialer:          dialer,
		Head:            make([]byte, 4),
		chClose:         make(chan util.Empty),
		sessionMap:      make(map[uint64]*rpcSession),
		asyncHandlerMap: make(map[uint64]*asyncHandler),
		streamLocalMap:  make(map[uint64]*Stream),
		streamRemoteMap: make(map[uint64]*Stream),
	}
	if c.Handler.AsyncWrite() && !c.Handler.AsyncWritev() {
		c.chSend = make(chan *Message, handler.SendQueueSize())
	}

	c.run()

	log.Info("%v\t%v\tConnected", c.Handler.LogTag(), conn.RemoteAddr())

	return c, nil
}

// ClientPool is a fixed set of client-role Clients.
type ClientPool struct {
	size    uint64
	round   uint64
	clients []*Client
}

// Size returns the number of Clients.
func (pool *ClientPool) Size() int {
	return len(pool.clients)
}

// Get returns the Client at index, modulo the pool size.
func (pool *ClientPool) Get(index int) *Client {
	return pool.clients[uint64(index)%pool.size]
}

// Next returns the next Client in round robin, skipping the ones stopped or
// reconnecting. If all of them are, it returns the last one tried.
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

// Handler returns the Handler of the next Client, which all Clients share
// when the pool is created with one Handler.
func (pool *ClientPool) Handler() Handler {
	return pool.Next().Handler
}

// Stop stops all Clients.
func (pool *ClientPool) Stop() {
	for _, c := range pool.clients {
		c.Stop()
	}
}

// NewClientPool creates a ClientPool of size Clients dialed with dialer and
// sharing one Handler: args[0] if it is a Handler, or else a clone of
// DefaultHandler. If any dial fails, the created Clients are stopped and the
// error is returned.
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

// NewClientPoolFromDialers is like NewClientPool, with one Client per dialer.
// It returns ErrClientInvalidPoolDialers if dialers is empty.
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

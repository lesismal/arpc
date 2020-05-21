package arpc

import (
	"encoding/binary"
	"errors"
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

// Client defines rpc client struct
type Client struct {
	mux sync.RWMutex

	running      bool
	reconnecting bool
	chSend       chan Message

	seq             uint64
	sessionMap      map[uint64]*rpcSession
	asyncHandlerMap map[uint64]func(*Context)

	onStop      func() int64
	onConnected func(*Client)

	Conn    net.Conn
	Reader  io.Reader
	Head    Header
	Codec   Codec
	Handler Handler
	Dialer  func() (net.Conn, error)
}

// OnConnected registers callback on connected
func (c *Client) OnConnected(onConnected func(*Client)) {
	c.onConnected = onConnected
}

// Run client
func (c *Client) Run() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.running {
		c.running = true
		c.chSend = make(chan Message, c.Handler.SendQueueSize())
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
		close(c.chSend)
		if c.onStop != nil {
			c.onStop()
		}
	}
}

// Call make rpc call
func (c *Client) Call(method string, req interface{}, rsp interface{}, timeout time.Duration) error {
	if !c.running {
		return ErrClientStopped
	}

	if timeout <= 0 {
		return fmt.Errorf("invalid timeout arg: %v", timeout)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	msg := c.newReqMessage(method, req, 0)
	seq := msg.Seq()

	session := &rpcSession{seq: seq, done: make(chan Message, 1)}
	c.addSession(seq, session)
	defer c.deleteSession(seq)

	select {
	case c.chSend <- msg:
	case <-timer.C:
		return ErrClientTimeout
	}

	select {
	case msg = <-session.done:
		defer memPut(msg)
	case <-timer.C:
		return ErrClientTimeout
	}

	switch msg.Cmd() {
	case RPCCmdRsp:
		switch vt := rsp.(type) {
		case *string:
			*vt = string(msg[HeadLen:])
		case *[]byte:
			*vt = msg[HeadLen:]
		case *error:
			*vt = errors.New(bytesToStr(msg[HeadLen:]))
		default:
			return c.Codec.Unmarshal(msg[HeadLen:], rsp)
		}
	case RPCCmdErr:
		return errors.New(string(msg[HeadLen:]))
	default:
	}

	return nil
}

// CallAsync make async rpc call
func (c *Client) CallAsync(method string, req interface{}, handler func(*Context), timeout time.Duration) error {
	msg := c.newReqMessage(method, req, 1)
	if handler != nil {
		seq := msg.Seq()
		c.addAsyncHandler(seq, handler)
	}

	return c.PushMsg(msg, timeout)
}

// Notify make rpc notify
func (c *Client) Notify(method string, data interface{}, timeout time.Duration) error {
	return c.CallAsync(method, data, nil, timeout)
}

// PushMsg push msg to client's send queue
func (c *Client) PushMsg(msg Message, timeout time.Duration) error {
	if !c.running {
		return ErrClientStopped
	}
	if c.reconnecting {
		return ErrClientReconnecting
	}

	switch timeout {
	case TimeForever:
		c.chSend <- msg
		msg.Retain()
	case TimeZero:
		select {
		case c.chSend <- msg:
			msg.Retain()
		default:
			return ErrClientQueueIsFull
		}
	default:
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case c.chSend <- msg:
			msg.Retain()
		case <-timer.C:
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

func (c *Client) addAsyncHandler(seq uint64, handler func(*Context)) {
	c.mux.Lock()
	c.asyncHandlerMap[seq] = handler
	c.mux.Unlock()
}

func (c *Client) getAndDeleteAsyncHandler(seq uint64) (func(*Context), bool) {
	c.mux.Lock()
	handler, ok := c.asyncHandlerMap[seq]
	if ok {
		delete(c.asyncHandlerMap, seq)
	}
	c.mux.Unlock()
	return handler, ok
}

func (c *Client) recvLoop() {
	var (
		err  error
		msg  Message
		addr = c.Conn.RemoteAddr()
	)

	// DefaultLogger.Info("[ARPC] Client: \"%v\" recvLoop start", c.Conn.RemoteAddr())
	// defer DefaultLogger.Info("[ARPC] Client: \"%v\" recvLoop stop", c.Conn.RemoteAddr())

	if c.Dialer == nil {
		for {
			msg, err = c.Handler.Recv(c)
			if err != nil {
				DefaultLogger.Info("[ARPC] Client \"%v\" Disconnected: %v", addr, err)
				c.Stop()
				return
			}
			c.Handler.OnMessage(c, msg)
		}
	} else {
		for c.running {
			for {
				msg, err = c.Handler.Recv(c)
				if err != nil {
					DefaultLogger.Info("[ARPC] Client \"%v\" Disconnected: %v", addr, err)
					break
				}
				c.Handler.OnMessage(c, msg)
			}

			c.reconnecting = true
			c.Conn.Close()
			c.Conn = nil

			for {
				DefaultLogger.Info("[ARPC] Client \"%v\" Reconnecting ...", addr)
				c.Conn, err = c.Dialer()
				if err == nil {
					DefaultLogger.Info("[ARPC] Client \"%v\" Connected", addr)
					c.Reader = c.Handler.WrapReader(c.Conn)

					c.reconnecting = false

					if c.onConnected != nil {
						go safe(func() {
							c.onConnected(c)
						})
					}

					break
				}

				time.Sleep(time.Second)
			}
		}
	}

}

func (c *Client) sendLoop() {
	// DefaultLogger.Info("[Arpc】 Client: %v sendLoop start", c.Conn.RemoteAddr())
	// defer DefaultLogger.Info("[ARPC] Client: %v sendLoop stop", c.Conn.RemoteAddr())
	var conn net.Conn
	for msg := range c.chSend {
		conn = c.Conn
		if !c.reconnecting {
			c.Handler.Send(conn, msg.Payload())
			msg.Release()
		}
	}
}

func (c *Client) newReqMessage(method string, req interface{}, async byte) Message {
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

	msg[headerIndexCmd] = RPCCmdReq
	msg[headerIndexAsync] = async
	msg[headerIndexMethodLen] = byte(len(method))
	copy(msg[HeadLen:HeadLen+len(method)], method)
	copy(msg[HeadLen+len(method):], data)

	return msg
}

// newClientWithConn factory
func newClientWithConn(conn net.Conn, codec Codec, handler Handler, onStop func() int64) *Client {
	DefaultLogger.Info("[ARPC] New Client \"%v\" Connected", conn.RemoteAddr())

	client := clientPool.Get().(*Client)
	client.Conn = conn
	client.Reader = handler.WrapReader(conn)
	client.Head = newHeader()
	client.Codec = codec
	client.Handler = handler
	client.sessionMap = make(map[uint64]*rpcSession)
	client.asyncHandlerMap = make(map[uint64]func(*Context))
	client.onStop = onStop

	return client
}

// NewClient factory
func NewClient(dialer func() (net.Conn, error)) (*Client, error) {
	conn, err := dialer()
	if err != nil {
		return nil, err
	}

	DefaultLogger.Info("[ARPC] Client \"%v\" Connected", conn.RemoteAddr())

	client := clientPool.Get().(*Client)
	client.Conn = conn
	client.Reader = DefaultHandler.WrapReader(conn)
	client.Head = newHeader()
	client.Codec = DefaultCodec
	client.Handler = DefaultHandler
	client.Dialer = dialer
	client.sessionMap = make(map[uint64]*rpcSession)
	client.asyncHandlerMap = make(map[uint64]func(*Context))

	return client, nil
}

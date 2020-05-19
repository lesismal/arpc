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
	mux          sync.RWMutex
	Conn         net.Conn
	Head         Header
	Handler      Handler
	Dialer       func() (net.Conn, error)
	Codec        Codec
	sessionMap   map[uint64]*rpcSession
	seq          uint64
	chSend       chan Message
	running      bool
	reconnecting bool
	onStop       func() int64
	onConnected  func(*Client)
	Reader       io.Reader
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

	msg := c.newReqMessage(method, req)
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

// Notify make rpc notify
func (c *Client) Notify(method string, data interface{}, timeout time.Duration) error {
	if !c.running {
		return ErrClientStopped
	}

	if timeout <= 0 {
		return fmt.Errorf("invalid timeout arg: %v", timeout)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	msg := c.newReqMessage(method, data)

	select {
	case c.chSend <- msg:
	case <-timer.C:
		return ErrClientTimeout
	}

	return nil
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
	case TimeZero:
		select {
		case c.chSend <- msg:
		default:
			return ErrClientQueueIsFull
		}
	default:
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case c.chSend <- msg:
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
				DefaultLogger.Info("[ARPC] Client \"%v\" Disconnected", addr)
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
			c.Handler.Send(conn, msg)
		}
	}
}

func (c *Client) newReqMessage(method string, req interface{}) Message {
	var (
		data    []byte
		msg     Message
		bodyLen int
	)

	data = valueToBytes(c.Codec, req)

	bodyLen = len(method) + len(data)

	msg = Message(make([]byte, HeadLen+bodyLen))
	binary.LittleEndian.PutUint32(msg[:4], uint32(bodyLen))
	binary.LittleEndian.PutUint64(msg[8:16], atomic.AddUint64(&c.seq, 1))

	msg[4] = RPCCmdReq
	msg[5] = byte(len(method))
	copy(msg[HeadLen:HeadLen+len(method)], method)
	copy(msg[HeadLen+len(method):], data)

	return msg
}

// newClientWithConn factory
func newClientWithConn(conn net.Conn, codec Codec, handler Handler, onStop func() int64) *Client {
	DefaultLogger.Info("[ARPC] New Client \"%v\" Connected", conn.RemoteAddr())

	return &Client{
		Conn:       conn,
		Reader:     handler.WrapReader(conn),
		Head:       newHeader(),
		Codec:      codec,
		Handler:    handler,
		onStop:     onStop,
		sessionMap: make(map[uint64]*rpcSession),
		seq:        0,
		chSend:     make(chan Message, 1024*8),
		running:    false,
	}
}

// NewClient factory
func NewClient(dialer func() (net.Conn, error)) (*Client, error) {
	conn, err := dialer()
	if err != nil {
		return nil, err
	}

	DefaultLogger.Info("[ARPC] Client \"%v\" Connected", conn.RemoteAddr())

	return &Client{
		Conn:       conn,
		Reader:     DefaultHandler.WrapReader(conn),
		Head:       newHeader(),
		Handler:    DefaultHandler,
		Dialer:     dialer,
		Codec:      DefaultCodec,
		sessionMap: make(map[uint64]*rpcSession, 1024*4),
		seq:        0,
		chSend:     make(chan Message, 1024*8),
		running:    false,
	}, nil
}

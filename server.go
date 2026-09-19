// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

// Server is an arpc server. Each accepted conn is served by a Client in the
// server role, sharing the Server's Codec and Handler.
type Server struct {
	// Accepted counts the accepted conns (not counting those rejected by
	// MaxLoad). It is updated without atomics by the accept loop.
	Accepted int64
	// CurrLoad is the number of current conns.
	CurrLoad int64
	// MaxLoad is the max number of concurrent conns; new conns beyond it are
	// closed at once. <= 0 means unlimited.
	MaxLoad int64

	// seq is kept 64-bit aligned on 32-bit platforms.
	seq uint64

	// Codec encodes and decodes message bodies.
	Codec codec.Codec
	// Handler handles the messages and events of all conns.
	Handler Handler

	// Listener is the listener being served.
	Listener net.Listener

	mux sync.Mutex

	// running is 1 while serving, 0 otherwise. It is accessed atomically since
	// Stop/Shutdown write it while runLoop reads it.
	running int32
	chStop  chan error
	clients map[*Client]util.Empty
}

func (s *Server) setRunning(v bool) {
	if v {
		atomic.StoreInt32(&s.running, 1)
	} else {
		atomic.StoreInt32(&s.running, 0)
	}
}

func (s *Server) isRunning() bool {
	return atomic.LoadInt32(&s.running) == 1
}

// Serve accepts conns on ln and serves them. It blocks until the Server is
// stopped or Accept fails with a non-temporary error, which it returns.
func (s *Server) Serve(ln net.Listener) error {
	// Stop/Shutdown read Listener and chStop from another goroutine, so set them
	// under the mutex.
	s.mux.Lock()
	s.Listener = ln
	s.chStop = make(chan error)
	s.mux.Unlock()
	log.Info("%v Running On: \"%v\"", s.Handler.LogTag(), ln.Addr())
	defer log.Info("%v Stopped", s.Handler.LogTag())
	return s.runLoop()
}

// Run listens on the TCP address addr and serves it, see Serve.
func (s *Server) Run(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Info("%v Running failed: %v", s.Handler.LogTag(), err)
		return err
	}
	s.mux.Lock()
	s.Listener = ln
	s.chStop = make(chan error)
	s.mux.Unlock()
	log.Info("%v Running On: \"%v\"", s.Handler.LogTag(), ln.Addr())
	// defer log.Info("%v Stopped", s.Handler.LogTag())
	return s.runLoop()
}

// Broadcast sends a notify to all conns. It does not block: the message is
// dropped for a conn whose send queue is full (see Handler.HandleOverstock).
// args[0], if any, must be a map[interface{}]interface{} of message values.
func (s *Server) Broadcast(method string, v interface{}, args ...interface{}) {
	msg := s.NewMessage(CmdNotify, method, v, args...)
	s.mux.Lock()
	defer func() {
		msg.Release()
		s.mux.Unlock()
	}()

	for c := range s.clients {
		msg.Retain()
		c.PushMsg(msg, TimeZero)
	}
}

// BroadcastWithFilter is like Broadcast, but only sends to the conns for which
// filter returns true. A nil filter matches all.
func (s *Server) BroadcastWithFilter(method string, v interface{}, filter func(*Client) bool, args ...interface{}) {
	msg := s.NewMessage(CmdNotify, method, v, args...)
	s.mux.Lock()
	defer func() {
		msg.Release()
		s.mux.Unlock()
	}()

	for c := range s.clients {
		if filter == nil || filter(c) {
			msg.Retain()
			c.PushMsg(msg, TimeZero)
		}
	}
}

// ForEach calls h for each conn. It holds the Server's lock, so h must not
// call methods of the Server that lock it.
func (s *Server) ForEach(h func(*Client)) {
	s.mux.Lock()
	defer s.mux.Unlock()
	for c := range s.clients {
		h(c)
	}
}

// ForEachWithFilter is like ForEach, but only calls h for the conns for which
// filter returns true. A nil filter matches all.
func (s *Server) ForEachWithFilter(h func(*Client), filter func(*Client) bool) {
	s.mux.Lock()
	defer s.mux.Unlock()
	for c := range s.clients {
		if filter == nil || filter(c) {
			h(c)
		}
	}
}

// Stop closes the listener and returns immediately, without waiting for the
// accept loop to exit; use Shutdown to wait. When the loop exits, all conns
// are stopped.
func (s *Server) Stop() error {
	s.setRunning(false)
	s.mux.Lock()
	ln := s.Listener
	chStop := s.chStop
	s.mux.Unlock()
	defer log.Info("%v \"%v\" Stop", s.Handler.LogTag(), ln.Addr())
	ln.Close()
	select {
	case <-chStop:
	case <-time.After(time.Second):
		return ErrTimeout
	default:
	}
	return nil
}

// Shutdown closes the listener and waits for the accept loop to exit and stop
// all conns. It returns ErrTimeout if ctx is done first.
func (s *Server) Shutdown(ctx context.Context) error {
	s.setRunning(false)
	s.mux.Lock()
	ln := s.Listener
	chStop := s.chStop
	s.mux.Unlock()
	defer log.Info("%v \"%v\" Shutdown", s.Handler.LogTag(), ln.Addr())
	ln.Close()
	select {
	case <-chStop:
	case <-ctx.Done():
		return ErrTimeout
	}
	return nil
}

// NewMessage creates a Message with the Server's Handler and Codec and a new
// sequence number. args[0], if any, must be a map[interface{}]interface{} of
// message values.
func (s *Server) NewMessage(cmd byte, method string, v interface{}, args ...interface{}) *Message {
	if len(args) == 0 {
		return newMessage(cmd, method, v, false, false, atomic.AddUint64(&s.seq, 1), s.Handler, s.Codec, nil)
	}
	return newMessage(cmd, method, v, false, false, atomic.AddUint64(&s.seq, 1), s.Handler, s.Codec, args[0].(map[interface{}]interface{}))
}

func (s *Server) addLoad() int64 {
	return atomic.AddInt64(&s.CurrLoad, 1)
}

func (s *Server) subLoad() int64 {
	return atomic.AddInt64(&s.CurrLoad, -1)
}

func (s *Server) addClient(c *Client) {
	s.mux.Lock()
	s.clients[c] = util.Empty{}
	s.mux.Unlock()
}

func (s *Server) deleteClient(c *Client) {
	s.mux.Lock()
	delete(s.clients, c)
	s.mux.Unlock()
}

// clearClients stops all conns asynchronously and empties the client set.
func (s *Server) clearClients() {
	s.mux.Lock()
	for c := range s.clients {
		go c.Stop()
	}
	s.clients = map[*Client]util.Empty{}
	s.mux.Unlock()
}

// runLoop accepts conns until the Server stops or Accept fails with a
// non-temporary error; temporary errors are retried after 50ms. On exit it
// stops all conns and closes chStop.
func (s *Server) runLoop() error {
	var (
		err  error
		cli  *Client
		conn net.Conn
	)

	s.setRunning(true)
	defer func() {
		s.clearClients()
		close(s.chStop)
	}()

	for s.isRunning() {
		conn, err = s.Listener.Accept()
		if err == nil {
			load := s.addLoad()
			if s.MaxLoad <= 0 || load <= s.MaxLoad {
				s.Accepted++
				cli = newClientWithConn(conn, s.Codec, s.Handler, func(c *Client) {
					s.deleteClient(c)
					s.subLoad()
				})
				s.addClient(cli)
				s.Handler.OnConnected(cli)
			} else {
				conn.Close()
				s.subLoad()
			}
		} else if s.isRunning() {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				log.Error("%v Accept error: %v; retrying...", s.Handler.LogTag(), err)
				time.Sleep(time.Second / 20)
			} else {
				log.Error("%v Accept error: %v", s.Handler.LogTag(), err)
				break
			}
		}
	}

	return err
}

// NewServer creates a Server with DefaultCodec and a clone of DefaultHandler.
func NewServer() *Server {
	h := DefaultHandler.Clone()
	h.SetLogTag("[ARPC SVR]")
	return &Server{
		Codec:   codec.DefaultCodec,
		Handler: h,
		clients: map[*Client]util.Empty{},
	}
}

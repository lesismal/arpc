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

// Server represents an arpc Server.
type Server struct {
	Accepted int64
	CurrLoad int64
	MaxLoad  int64

	// 64-aligned on 32-bit
	seq uint64

	Codec   codec.Codec
	Handler Handler

	Listener net.Listener

	mux sync.Mutex

	// running is accessed atomically(runLoop reads it while Stop/Shutdown
	// write it from another goroutine), 0 means stopped, 1 means running.
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

// Serve starts service with listener.
func (s *Server) Serve(ln net.Listener) error {
	// Listener/chStop are read by Stop/Shutdown from another goroutine, so they
	// must be published under the mutex.
	s.mux.Lock()
	s.Listener = ln
	s.chStop = make(chan error)
	s.mux.Unlock()
	log.Info("%v Running On: \"%v\"", s.Handler.LogTag(), ln.Addr())
	defer log.Info("%v Stopped", s.Handler.LogTag())
	return s.runLoop()
}

// Run starts tcp service on addr.
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

func (s *Server) ForEach(h func(*Client)) {
	s.mux.Lock()
	defer s.mux.Unlock()
	for c := range s.clients {
		h(c)
	}
}

func (s *Server) ForEachWithFilter(h func(*Client), filter func(*Client) bool) {
	s.mux.Lock()
	defer s.mux.Unlock()
	for c := range s.clients {
		if filter == nil || filter(c) {
			h(c)
		}
	}
}

// Stop stops service.
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

// Shutdown shutdown service.
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

// NewMessage creates a Message.
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

func (s *Server) clearClients() {
	s.mux.Lock()
	for c := range s.clients {
		go c.Stop()
	}
	s.clients = map[*Client]util.Empty{}
	s.mux.Unlock()
}

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

// NewServer creates an arpc Server.
func NewServer() *Server {
	h := DefaultHandler.Clone()
	h.SetLogTag("[ARPC SVR]")
	return &Server{
		Codec:   codec.DefaultCodec,
		Handler: h,
		clients: map[*Client]util.Empty{},
	}
}

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

// Server definition
type Server struct {
	Accepted int64
	CurrLoad int64
	MaxLoad  int64

	Codec   codec.Codec
	Handler Handler

	Listener net.Listener

	mux sync.Mutex

	seq     uint64
	running bool
	chStop  chan error
	clients map[*Client]util.Empty
}

// Serve starts rpc service with listener
func (s *Server) Serve(ln net.Listener) error {
	s.Listener = ln
	s.chStop = make(chan error)
	log.Info("%v Running On: \"%v\"", s.Handler.LogTag(), ln.Addr())
	defer log.Info("%v Stopped", s.Handler.LogTag())
	return s.runLoop()
}

// Run starts a tcp service on addr
func (s *Server) Run(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Info("%v Running failed: %v", s.Handler.LogTag(), err)
		return err
	}
	s.Listener = ln
	s.chStop = make(chan error)
	log.Info("%v Running On: \"%v\"", s.Handler.LogTag(), ln.Addr())
	// defer log.Info("%v Stopped", s.Handler.LogTag())
	return s.runLoop()
}

// Stop rpc service
func (s *Server) Stop() error {
	defer log.Info("%v %v Stop", s.Handler.LogTag(), s.Listener.Addr())
	s.running = false
	s.Listener.Close()
	select {
	case <-s.chStop:
	case <-time.After(time.Second):
		return ErrTimeout
	default:
	}
	return nil
}

// Shutdown stop rpc service
func (s *Server) Shutdown(ctx context.Context) error {
	defer log.Info("%v %v Shutdown", s.Handler.LogTag(), s.Listener.Addr())
	s.running = false
	s.Listener.Close()
	select {
	case <-s.chStop:
	case <-ctx.Done():
		return ErrTimeout
	}
	return nil
}

// NewMessage factory
func (s *Server) NewMessage(cmd byte, method string, v interface{}) *Message {
	return newMessage(cmd, method, v, false, false, atomic.AddUint64(&s.seq, 1), s.Handler, s.Codec, nil)
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

	s.running = true
	defer func() {
		s.clearClients()
		close(s.chStop)
	}()

	for s.running {
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
		} else {
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

// NewServer factory
func NewServer() *Server {
	h := DefaultHandler.Clone()
	h.SetLogTag("[ARPC SVR]")
	return &Server{
		Codec:   codec.DefaultCodec,
		Handler: h,
		clients: map[*Client]util.Empty{},
	}
}

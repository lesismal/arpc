// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"net"
	"sync/atomic"
	"time"
)

// Server definition
type Server struct {
	ln       net.Listener
	chStop   chan error
	running  bool
	Accepted int64
	CurrLoad int64
	MaxLoad  int64

	Codec   Codec
	Handler Handler
}

func (s *Server) addLoad() int64 {
	return atomic.AddInt64(&s.CurrLoad, 1)
}

func (s *Server) subLoad() int64 {
	return atomic.AddInt64(&s.CurrLoad, -1)
}

func (s *Server) runLoop() error {
	var (
		err       error
		cli       *Client
		conn      net.Conn
		tempDelay time.Duration
	)

	s.running = true
	defer close(s.chStop)

	for s.running {
		conn, err = s.ln.Accept()
		if err == nil {
			load := s.addLoad()
			if s.MaxLoad == 0 || load <= s.MaxLoad {
				s.Accepted++
				cli = newClientWithConn(conn, s.Codec, s.Handler, s.subLoad)
				cli.Run()
			} else {
				conn.Close()
				s.subLoad()
			}
		} else {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				DefaultLogger.Info("[ARPC SVR] Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
			} else {
				DefaultLogger.Error("[ARPC SVR] Accept error:", err)
				break
			}
		}
	}

	return err
}

// Serve starts rpc service with listener
func (s *Server) Serve(ln net.Listener) error {
	s.ln = ln
	s.chStop = make(chan error)
	DefaultLogger.Info("[ARPC SVR] Running On: \"%v\"", ln.Addr())
	defer DefaultLogger.Info("[ARPC SVR] Stopped")
	return s.runLoop()
}

// Run starts a tcp service on addr
func (s *Server) Run(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.chStop = make(chan error)
	DefaultLogger.Info("[ARPC SVR] Running On: \"%v\"", ln.Addr())
	defer DefaultLogger.Info("[ARPC SVR] Stopped")
	return s.runLoop()
}

// Shutdown stop rpc service
func (s *Server) Shutdown(timeout time.Duration) error {
	DefaultLogger.Info("[ARPC SVR] %v Shutdown...", s.ln.Addr())
	s.running = false
	s.ln.Close()
	select {
	case <-s.chStop:
	case <-time.After(timeout):
		return ErrTimeout
	}
	return nil
}

// NewServer factory
func NewServer() *Server {
	return &Server{
		Codec:   DefaultCodec,
		Handler: DefaultHandler.Clone(),
	}
}

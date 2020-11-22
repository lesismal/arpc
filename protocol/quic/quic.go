// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package quic

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	quic "github.com/lucas-clemente/quic-go"
)

// Listener wraps quick.Listener to net.Listener
type Listener struct {
	quic.Listener
}

// Accept waits for and returns the next connection to the listener.
func (ln *Listener) Accept() (net.Conn, error) {
	session, err := ln.Listener.Accept(context.Background())
	if err != nil {
		return nil, err
	}

	stream, err := session.AcceptStream(context.Background())
	if err != nil {
		return nil, err
	}

	return &Conn{session, stream}, err
}

// Conn wraps quick.Session to net.Conn
type Conn struct {
	quic.Session
	quic.Stream
}

// Listen wraps quic listen
func Listen(addr string, config *tls.Config) (net.Listener, error) {
	ln, err := quic.ListenAddr(addr, config, nil)
	if err != nil {
		return nil, err
	}
	return &Listener{ln}, err
}

// Dial wraps quic dial
func Dial(addr string, tlsConf *tls.Config, quicConf *quic.Config, timeout time.Duration) (net.Conn, error) {
	var (
		ctx    = context.Background()
		cancel func()
	)
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	}

	session, err := quic.DialAddr(addr, tlsConf, quicConf)
	if err != nil {
		return nil, err
	}

	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	return &Conn{session, stream}, err
}

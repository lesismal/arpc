// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_HandleReconnect(t *testing.T) {
	const addr = "localhost:11008"

	svr := NewServer()
	chSvrCli := make(chan *Client, 4)
	svr.Handler.HandleConnected(func(c *Client) {
		chSvrCli <- c
	})
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	go svr.Serve(ln)
	defer svr.Stop()

	// The initial dial succeeds, the next two(the first reconnect attempts)
	// fail, and later ones succeed.
	const failTimes = 2
	errDial := errors.New("mock dial error")
	var dials int32
	dialer := func() (net.Conn, error) {
		n := atomic.AddInt32(&dials, 1)
		if n > 1 && n <= 1+failTimes {
			return nil, errDial
		}
		return net.DialTimeout("tcp", addr, time.Second)
	}

	h := NewHandler()
	chInfo := make(chan *ReconnectInfo, 8)
	h.HandleReconnect(func(c *Client, info *ReconnectInfo) {
		if info.Success && c.Conn == nil {
			t.Errorf("c.Conn should be the new conn on success")
		}
		chInfo <- info
	})
	c, err := NewClient(dialer, h)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	var svrCli *Client
	select {
	case svrCli = <-chSvrCli:
	case <-time.After(3 * time.Second):
		t.Fatalf("server-side client not connected")
	}
	targetAddr := c.Conn.RemoteAddr().String()
	// Drop the connection from the server side to trigger reconnect.
	svrCli.Conn.Close()

	for i := 1; i <= failTimes+1; i++ {
		select {
		case info := <-chInfo:
			if info.Times != i {
				t.Fatalf("attempt %d: Times = %d", i, info.Times)
			}
			if info.Addr != targetAddr {
				t.Fatalf("attempt %d: Addr = %q, want %q", i, info.Addr, targetAddr)
			}
			if info.MaxTimes != h.MaxReconnectTimes() {
				t.Fatalf("attempt %d: MaxTimes = %d", i, info.MaxTimes)
			}
			wantSuccess := i > failTimes
			if info.Success != wantSuccess {
				t.Fatalf("attempt %d: Success = %v, want %v", i, info.Success, wantSuccess)
			}
			if wantSuccess && info.Err != nil {
				t.Fatalf("attempt %d: Err = %v, want nil", i, info.Err)
			}
			if !wantSuccess && info.Err != errDial {
				t.Fatalf("attempt %d: Err = %v, want %v", i, info.Err, errDial)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("attempt %d: OnReconnect not called", i)
		}
	}
}

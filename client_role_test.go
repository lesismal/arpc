// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"net"
	"testing"
	"time"
)

func TestClient_IsClientIsServer(t *testing.T) {
	const addr = "localhost:11007"

	svr := NewServer()
	// Capture the server-side Client(created for an accepted connection).
	chSvrCli := make(chan *Client, 1)
	svr.Handler.HandleConnected(func(c *Client) {
		select {
		case chSvrCli <- c:
		default:
		}
	})
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	go svr.Serve(ln)
	defer svr.Stop()

	cli, err := NewClient(func() (net.Conn, error) {
		return net.DialTimeout("tcp", addr, time.Second)
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer cli.Stop()

	// The dialing side is a client-side Client.
	if !cli.IsClient() || cli.IsServer() {
		t.Fatalf("dialing Client: IsClient()=%v IsServer()=%v, want true/false", cli.IsClient(), cli.IsServer())
	}

	// The accepted side is a server-side Client.
	var svrCli *Client
	select {
	case svrCli = <-chSvrCli:
	case <-time.After(time.Second * 3):
		t.Fatal("server did not accept a connection in time")
	}
	if svrCli.IsClient() || !svrCli.IsServer() {
		t.Fatalf("accepted Client: IsClient()=%v IsServer()=%v, want false/true", svrCli.IsClient(), svrCli.IsServer())
	}
}

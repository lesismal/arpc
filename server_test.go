// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"fmt"
	"log"
	"net"
	"testing"
	"time"
)

var testServerAddr = "localhost:12000"

func TestServer_addLoad(t *testing.T) {
	var (
		half  int64 = 10000
		total int64 = half * 2
	)

	s := NewServer()
	for i := int64(0); i < total; i++ {
		s.addLoad()
	}
	if s.CurrLoad != total {
		t.Fatalf("addLoad failed: %v != %v", s.CurrLoad, total)
	}
}

func TestServer_subLoad(t *testing.T) {
	var (
		half  int64 = 10000
		total int64 = half * 2
	)

	s := NewServer()
	for i := int64(0); i < total; i++ {
		s.addLoad()
	}
	if s.CurrLoad != total {
		t.Fatalf("addLoad failed: %v != %v", s.CurrLoad, total)
	}

	for i := int64(0); i < half; i++ {
		s.subLoad()
	}
	if s.CurrLoad != half {
		t.Fatalf("subLoad failed: %v != %v", s.CurrLoad, half)
	}

	for i := int64(0); i < half; i++ {
		s.subLoad()
	}
	if s.CurrLoad != 0 {
		t.Fatalf("subLoad failed: %v != %v", s.CurrLoad, half)
	}
}

func TestServer_NewMessage(t *testing.T) {
	s := &Server{}
	for cmd := byte(1); cmd <= 3; cmd++ {
		method := fmt.Sprintf("method_%v", cmd)
		message := fmt.Sprintf("message_%v", cmd)
		msg := s.NewMessage(cmd, method, message)
		if msg == nil {
			t.Fatalf("Server.NewMessage() = nil")
		}
		if msg.Cmd() != cmd {
			t.Fatalf("Server.NewMessage() error, cmd is: %v, want: %v", msg.Cmd(), cmd)
		}
		if msg.Method() != method {
			t.Fatalf("Server.NewMessage() error, cmd is: %v, want: %v", msg.Method(), method)
		}
		if msg.Method() != method {
			t.Fatalf("Server.NewMessage() error, cmd is: %v, want: %v", string(msg.Data()), message)
		}
	}
}

func TestServer_Serve(t *testing.T) {
	ln, err := net.Listen("tcp", testServerAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := NewServer()
	svr.MaxLoad = 3
	go svr.Serve(ln)
	time.Sleep(time.Second / 100)
	for i := 0; i < 3; i++ {
		if _, err := net.Dial("tcp", testServerAddr); err != nil {
			log.Fatalf("failed to Dial: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if conn, err := net.Dial("tcp", testServerAddr); err != nil {
			log.Fatalf("failed to Dial: %v", err)
		} else {
			conn.SetReadDeadline(time.Now().Add(time.Second / 10))
			if _, err = conn.Read([]byte{1}); err == nil {
				log.Fatalf("conn.Read success, should be closed by server(limited by MaxLoad)")
			}
		}
	}
	svr.Stop()
}

func TestServer_Run(t *testing.T) {
	svr := NewServer()
	go svr.Run(testServerAddr)
	go svr.Run(testServerAddr)
	time.Sleep(time.Second / 100)
	svr.Stop()
}

func TestServer_Shutdown(t *testing.T) {
	svr := NewServer()
	go svr.Run(":8899")
	time.Sleep(time.Second / 100)
	svr.Shutdown(context.Background())
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"log"
	"net"
	"testing"
	"time"
)

func TestServer_Service(t *testing.T) {
	addr := ":15678"

	s := NewServer()
	go s.Run(addr)

	time.Sleep(time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := s.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	s = NewServer()
	go s.Serve(ln)
	time.Sleep(time.Second)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = s.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

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

func TestServer_Serve(t *testing.T) {
	ln, err := net.Listen("tcp", ":8888")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svr := NewServer()
	tm := time.AfterFunc(time.Second, func() {
		t.Errorf("listener.Close timeout")
	})
	time.AfterFunc(time.Second/5, func() {
		ln.Close()
	})
	svr.MaxLoad = 3
	go func() {
		for i := 0; i < 5; i++ {
			net.Dial("tcp", "localhost:8888")
		}
	}()
	svr.Serve(ln)
	tm.Stop()
}

func TestServer_Run(t *testing.T) {
	svr := NewServer()
	tm := time.AfterFunc(time.Second, func() {
		t.Errorf("Server.Stop timeout")
	})
	time.AfterFunc(time.Second/10, func() {
		svr.Stop()
	})
	go svr.Run(":8888")
	svr.Run(":8888")
	tm.Stop()
}

func TestServer_Shutdown(t *testing.T) {
	svr := NewServer()
	tm := time.AfterFunc(time.Second, func() {
		t.Errorf("Server.Stop timeout")
	})
	time.AfterFunc(time.Second/10, func() {
		svr.Shutdown(context.Background())
	})
	svr.Run(":8888")
	tm.Stop()
}

// Copyright 2020 lesismal. All rights reserved.

// Use of this source code is governed by an MIT-style

// license that can be found in the LICENSE file.

package arpc

import (
	"net"
	"testing"
	"time"
)

func TestServer_Load(t *testing.T) {
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

func TestServer_Service(t *testing.T) {
	addr := ":8888"

	s := NewServer()
	go s.Run(addr)
	time.Sleep(time.Second)
	err := s.Shutdown(time.Second)
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
	err = s.Shutdown(time.Second)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

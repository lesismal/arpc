// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package websocket

import (
	"log"
	"net/http"
	"testing"
)

func TestAll(t *testing.T) {
	ln, _ := NewListener(":8888", nil)
	http.HandleFunc("/ws", ln.(*Listener).Handler)
	go func() {
		err := http.ListenAndServe(":8888", nil)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()

	str := "hello"

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer conn.Close()

		bread := make([]byte, len(str))
		_, err = conn.Read(bread)
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		conn.Write(bread)
	}()

	conn, err := Dial("ws://localhost:8888/ws")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer conn.Close()
	bread := make([]byte, len(str))
	nwrite, err := conn.Write([]byte(str))
	if err != nil || nwrite != len(str) {
		t.Fatalf("failed to listen: %v", err)
	}
	_, err = conn.Read(bread)
	if err != nil || string(bread) != str {
		t.Fatalf("failed to listen: %v", err)
	}
}

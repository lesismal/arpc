// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"testing"
	"unsafe"
)

func TestRefMessage(t *testing.T) {
	method := "/test/RefMessage/RetainAndRelease"
	payload := "payload data"
	refCount := 1024 * 128
	msg := NewRefMessage(CmdRequest, nil, method, payload)

	if methodStr := msg.Real().Method(); methodStr != method {
		t.Fatalf("NewRefMessage wrong method: %v != %v", methodStr, method)
	}
	if body := msg.Real().Body(); string(body) != payload {
		t.Fatalf("NewRefMessage wrong body: %v != %v", string(body), payload)
	}
	for i := 0; i < refCount; i++ {
		msg.Retain()
	}
	for i := 0; i < refCount; i++ {
		msg.Release()
	}
	pre := msg
	msg.Release()
	msg = NewRefMessage(CmdRequest, nil, method, payload)
	if unsafe.Pointer(&pre[0]) != unsafe.Pointer(&msg[0]) {
		t.Fatalf("NewRefMessage mem different: %v != %v", unsafe.Pointer(&pre[0]), unsafe.Pointer(&msg[0]))
	}
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"io"
	"net"
	"testing"
)

func Test_handler_Clone(t *testing.T) {
	if got := DefaultHandler.Clone(); got == nil {
		t.Errorf("handler.Clone() = nil")
	}
}

func Test_handler_LogTag(t *testing.T) {
	if got := DefaultHandler.LogTag(); got != "[ARPC CLI]" {
		t.Errorf("handler.LogTag() = %v, want %v", got, "[ARPC CLI]")
	}
}

func Test_handler_SetLogTag(t *testing.T) {
	logtag := "XYZ"
	DefaultHandler.SetLogTag(logtag)
	if got := DefaultHandler.LogTag(); got != logtag {
		t.Errorf("handler.LogTag() = %v, want %v", got, logtag)
	}
}

func Test_handler_HandleConnected(t *testing.T) {
	DefaultHandler.HandleConnected(func(*Client) {})
}

func Test_handler_OnConnected(t *testing.T) {
	DefaultHandler.OnConnected(nil)
}

func Test_handler_HandleDisconnected(t *testing.T) {
	DefaultHandler.HandleDisconnected(func(*Client) {})
}

func Test_handler_OnDisconnected(t *testing.T) {
	DefaultHandler.OnDisconnected(nil)
}

func Test_handler_HandleOverstock(t *testing.T) {
	DefaultHandler.HandleOverstock(func(c *Client, m *Message) {})
}

func Test_handler_OnOverstock(t *testing.T) {
	DefaultHandler.OnOverstock(nil, nil)
}

func Test_handler_HandleSessionMiss(t *testing.T) {
	DefaultHandler.HandleSessionMiss(func(c *Client, m *Message) {})
}

func Test_handler_OnSessionMiss(t *testing.T) {
	DefaultHandler.OnSessionMiss(nil, nil)
}

func Test_handler_BeforeRecv(t *testing.T) {
	DefaultHandler.BeforeRecv(func(net.Conn) error { return nil })
}

func Test_handler_BeforeSend(t *testing.T) {
	DefaultHandler.BeforeSend(func(net.Conn) error { return nil })
}

func Test_handler_BatchRecv(t *testing.T) {
	if got := DefaultHandler.BatchRecv(); got != true {
		t.Errorf("handler.BatchRecv() = %v, want %v", got, true)
	}
}

func Test_handler_SetBatchRecv(t *testing.T) {
	DefaultHandler.SetBatchRecv(false)
	if got := DefaultHandler.BatchRecv(); got != false {
		t.Errorf("handler.BatchRecv() = %v, want %v", got, false)
	}
}

func Test_handler_BatchSend(t *testing.T) {
	if got := DefaultHandler.BatchSend(); got != true {
		t.Errorf("handler.BatchSend() = %v, want %v", got, true)
	}
}

func Test_handler_SetBatchSend(t *testing.T) {
	DefaultHandler.SetBatchSend(false)
	if got := DefaultHandler.BatchSend(); got != false {
		t.Errorf("handler.BatchSend() = %v, want %v", got, false)
	}
}

func Test_handler_WrapReader(t *testing.T) {
	DefaultHandler.SetReaderWrapper(nil)
	if got := DefaultHandler.WrapReader(nil); got != nil {
		t.Errorf("handler.WrapReader() = %v, want %v", got, nil)
	}
}

func Test_handler_SetReaderWrapper(t *testing.T) {
	Test_handler_WrapReader(t)
}

func Test_handler_RecvBufferSize(t *testing.T) {
	if got := DefaultHandler.RecvBufferSize(); got != 8192 {
		t.Errorf("handler.RecvBufferSize() = %v, want %v", got, 8192)
	}
}

func Test_handler_SetRecvBufferSize(t *testing.T) {
	size := 1024
	DefaultHandler.SetRecvBufferSize(size)
	if got := DefaultHandler.RecvBufferSize(); got != size {
		t.Errorf("handler.RecvBufferSize() = %v, want %v", got, size)
	}
}

func Test_handler_SendQueueSize(t *testing.T) {
	if got := DefaultHandler.SendQueueSize(); got <= 0 {
		t.Errorf("handler.RecvBufferSize() = %v, want %v", got, 1024)
	}
}

func Test_handler_SetSendQueueSize(t *testing.T) {
	size := 2048
	DefaultHandler.SetSendQueueSize(size)
	if got := DefaultHandler.SendQueueSize(); got != size {
		t.Errorf("handler.RecvBufferSize() = %v, want %v", got, size)
	}
}

func Test_handler_Handle(t *testing.T) {
	DefaultHandler.Handle("/hello", func(*Context) {})
}

type registerReq struct{ A int }
type registerRsp struct{ B int }

// registerService holds several methods used to exercise Register:
//   - Add/AddBinding   : an eligible pair, should be registered.
//   - Sub              : matching signature but no Binding pair, should be skipped.
//   - Mul/MulBinding   : matching signature but Binding is not a HandlerFunc, skipped.
//   - Helper           : not a request/response method, ignored.
//   - lower/lowerBinding: unexported, ignored.
type registerService struct {
	addCalled bool
}

func (s *registerService) Add(ctx context.Context, req *registerReq, rsp *registerRsp) {
	s.addCalled = true
	rsp.B = req.A + 1
}

func (s *registerService) AddBinding(ctx *Context) {
	req := &registerReq{A: 1}
	rsp := &registerRsp{}
	s.Add(context.Background(), req, rsp)
}

// Sub has the right signature but no SubBinding pair: must be skipped.
func (s *registerService) Sub(ctx context.Context, req *registerReq, rsp *registerRsp) {}

// Mul has the right signature, but MulBinding is not an arpc.HandlerFunc: skipped.
func (s *registerService) Mul(ctx context.Context, req *registerReq, rsp *registerRsp) {}
func (s *registerService) MulBinding(ctx *Context) error                               { return nil }

// Helper is not a request/response method and must be ignored.
func (s *registerService) Helper() {}

func Test_handler_Register(t *testing.T) {
	h := NewHandler().(*handler)
	svc := &registerService{}

	if err := h.Register("Svc", svc); err != nil {
		t.Fatalf("handler.Register() error = %v", err)
	}

	// Only the Add/AddBinding pair should have been registered, using the
	// "Service.Method" route name.
	if _, ok := h.routes["Svc.Add"]; !ok {
		t.Fatalf("route %q not registered, routes = %v", "Svc.Add", h.routes)
	}
	if _, ok := h.routes["Svc.Sub"]; ok {
		t.Errorf("route %q should be skipped(no Binding pair)", "Svc.Sub")
	}
	if _, ok := h.routes["Svc.Mul"]; ok {
		t.Errorf("route %q should be skipped(Binding is not a HandlerFunc)", "Svc.Mul")
	}
	if _, ok := h.routes["Svc.Helper"]; ok {
		t.Errorf("route %q should not be registered", "Svc.Helper")
	}
	// routes always contains the reserved "" entry, plus the single Svc.Add.
	if len(h.routes) != 2 {
		t.Errorf("unexpected registered route count = %v, want %v, routes = %v", len(h.routes), 2, h.routes)
	}

	// The registered binding callback should invoke the first method.
	rh, ok := h.routes["Svc.Add"]
	if !ok {
		t.Fatalf("route %q not registered", "Svc.Add")
	}
	rh.handlers[len(rh.handlers)-1](&Context{})
	if !svc.addCalled {
		t.Errorf("registered binding did not call the first method")
	}
}

func Test_handler_Register_EmptyService(t *testing.T) {
	h := NewHandler().(*handler)
	if err := h.Register("", &registerService{}); err != nil {
		t.Fatalf("handler.Register() error = %v", err)
	}
	// With an empty service name the route is just the method name.
	if _, ok := h.routes["Add"]; !ok {
		t.Fatalf("route %q not registered, routes = %v", "Add", h.routes)
	}
}

// noPairService has no eligible method pair at all.
type noPairService struct{}

func (s *noPairService) Foo()      {}
func (s *noPairService) Bar(x int) {}

func Test_handler_Register_NoPairPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("handler.Register() should panic when no eligible method pair is found")
		}
	}()
	NewHandler().(*handler).Register("", &noPairService{})
}

func Test_handler_Register_Nil(t *testing.T) {
	if err := NewHandler().(*handler).Register("", nil); err == nil {
		t.Errorf("handler.Register(nil) should return an error")
	}
}

func TestRegister(t *testing.T) {
	d := DefaultHandler
	SetHandler(NewHandler())
	defer SetHandler(d)

	if err := Register("Svc", &registerService{}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, ok := DefaultHandler.(*handler).routes["Svc.Add"]; !ok {
		t.Errorf("route %q not registered via package-level Register", "Svc.Add")
	}
}

func TestNewHandler(t *testing.T) {
	if got := NewHandler(); got == nil {
		t.Errorf("NewHandler() = nil")
	}
}

func TestSetHandler(t *testing.T) {
	d := DefaultHandler
	h := NewHandler()
	SetHandler(h)
	SetLogTag("nothing")
	HandleConnected(func(*Client) {})
	HandleConnected(nil)
	HandleDisconnected(func(*Client) {})
	HandleDisconnected(nil)
	HandleOverstock(func(c *Client, m *Message) {})
	HandleMessageDropped(func(c *Client, m *Message) {})
	HandleSessionMiss(func(c *Client, m *Message) {})
	BeforeRecv(func(net.Conn) error { return nil })
	BeforeSend(func(net.Conn) error { return nil })
	SetBatchRecv(true)
	SetBatchSend(true)
	SetAsyncResponse(true)
	SetReaderWrapper(func(c net.Conn) io.Reader { return c })
	SetRecvBufferSize(4096)
	SetSendQueueSize(4096)
	Use(func(*Context) {})
	UseCoder(nil)
	Handle("nothing", func(*Context) {}, true)
	HandleNotFound(func(*Context) {})
	HandleMalloc(func(int) []byte { return nil })
	HandleFree(func([]byte) {})
	SetHandler(d)
}

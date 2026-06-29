// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/lesismal/arpc/codec"
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
//   - Add/AddBinding : an eligible pair, the Binding is registered as "Svc.Add".
//   - Sub            : first-method signature with no Binding pair, registered
//     standalone as "Svc.Sub" with an auto-generated handler.
//   - Mul/MulBinding : first-method signature but Binding is not a HandlerFunc,
//     so it is not a valid pair and Mul is registered standalone as "Svc.Mul".
//   - Helper         : not a request/response method, ignored.
type registerService struct {
	addCalled bool
	addGotA   int
	subCalled bool
	subGotA   int
}

func (s *registerService) Add(ctx context.Context, req *registerReq, rsp *registerRsp) {
	s.addCalled = true
	s.addGotA = req.A
	rsp.B = req.A + 1
}

func (s *registerService) AddBinding(ctx *Context) {
	req := &registerReq{}
	rsp := &registerRsp{}
	if err := ctx.Bind(req); err != nil {
		ctx.Error(err)
		return
	}
	// ctx implements context.Context, pass it through instead of a new one.
	s.Add(ctx, req, rsp)
}

// Sub has the first-method signature but no SubBinding pair: registered
// standalone with an auto-generated handler.
func (s *registerService) Sub(ctx context.Context, req *registerReq, rsp *registerRsp) {
	s.subCalled = true
	s.subGotA = req.A
	rsp.B = req.A - 1
}

// Mul has the first-method signature, but MulBinding is not an arpc.HandlerFunc,
// so the pair is invalid and Mul is registered standalone.
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

	// Add(paired), Sub and Mul(standalone first methods) are all registered.
	for _, name := range []string{"Svc.Add", "Svc.Sub", "Svc.Mul"} {
		if _, ok := h.routes[name]; !ok {
			t.Errorf("route %q not registered, routes = %v", name, h.routes)
		}
	}
	if _, ok := h.routes["Svc.Helper"]; ok {
		t.Errorf("route %q should not be registered", "Svc.Helper")
	}
	// "" + Svc.Add + Svc.Sub + Svc.Mul
	if len(h.routes) != 4 {
		t.Errorf("unexpected registered route count = %v, want %v, routes = %v", len(h.routes), 4, h.routes)
	}

	invoke := func(route string, a int) {
		rh, ok := h.routes[route]
		if !ok {
			t.Fatalf("route %q not registered", route)
		}
		ctx := &Context{
			Client:  &Client{Codec: codec.DefaultCodec, Handler: DefaultHandler},
			Message: newMessage(CmdRequest, route, &registerReq{A: a}, false, false, 0, DefaultHandler, codec.DefaultCodec, nil),
		}
		rh.handlers[len(rh.handlers)-1](ctx)
	}

	// The paired binding should bind the request and invoke Add.
	invoke("Svc.Add", 1)
	if !svc.addCalled || svc.addGotA != 1 {
		t.Errorf("paired binding failed: addCalled = %v, addGotA = %v", svc.addCalled, svc.addGotA)
	}

	// The auto-generated handler for the unpaired Sub should bind and invoke it.
	invoke("Svc.Sub", 7)
	if !svc.subCalled || svc.subGotA != 7 {
		t.Errorf("standalone first-method handler failed: subCalled = %v, subGotA = %v", svc.subCalled, svc.subGotA)
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

// standaloneService mixes a paired method with a standalone HandlerFunc method.
type standaloneService struct {
	pinged bool
}

func (s *standaloneService) Echo(ctx context.Context, req *registerReq, rsp *registerRsp) {
	rsp.B = req.A
}
func (s *standaloneService) EchoBinding(ctx *Context) {}

// Ping is a standalone arpc.HandlerFunc method without a paired first method,
// so it should be registered under its own name.
func (s *standaloneService) Ping(ctx *Context) { s.pinged = true }

func Test_handler_Register_Standalone(t *testing.T) {
	h := NewHandler().(*handler)
	svc := &standaloneService{}
	if err := h.Register("Svc", svc); err != nil {
		t.Fatalf("handler.Register() error = %v", err)
	}

	// Paired method is registered under the first method's name.
	if _, ok := h.routes["Svc.Echo"]; !ok {
		t.Errorf("paired route %q not registered, routes = %v", "Svc.Echo", h.routes)
	}
	// Standalone HandlerFunc method is registered under its own name.
	rh, ok := h.routes["Svc.Ping"]
	if !ok {
		t.Fatalf("standalone route %q not registered, routes = %v", "Svc.Ping", h.routes)
	}
	// The consumed Binding method must not also be registered standalone.
	if _, ok := h.routes["Svc.EchoBinding"]; ok {
		t.Errorf("consumed binding %q should not be registered standalone", "Svc.EchoBinding")
	}
	// "" + Svc.Echo + Svc.Ping
	if len(h.routes) != 3 {
		t.Errorf("unexpected route count = %v, want %v, routes = %v", len(h.routes), 3, h.routes)
	}

	// The standalone route should invoke the method itself.
	rh.handlers[len(rh.handlers)-1](&Context{})
	if !svc.pinged {
		t.Errorf("standalone route did not invoke the method")
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

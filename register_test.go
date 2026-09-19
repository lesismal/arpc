// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"strings"
	"testing"
	"time"
)

type calcReq struct{ A, B int }
type calcRsp struct{ Sum int }

// calcService exercises every kind of method Register handles:
//   - Add/AddBinding: an eligible pair, AddBinding is registered as "Add".
//   - Sub: the request/response signature without a Binding, registered with
//     an auto-generated handler.
//   - Mul/MulBinding: MulBinding is not a HandlerFunc, so Mul is registered
//     with an auto-generated handler, and MulBinding is ignored.
//   - Ping: a HandlerFunc method, registered as is.
//   - Helper, Other, unexported: not eligible, ignored.
type calcService struct {
	bindings int
}

func (s *calcService) Add(ctx context.Context, req *calcReq, rsp *calcRsp) {
	rsp.Sum = req.A + req.B
}

func (s *calcService) AddBinding(ctx *Context) {
	s.bindings++
	req, rsp := &calcReq{}, &calcRsp{}
	if err := ctx.Bind(req); err != nil {
		ctx.Error(err)
		return
	}
	s.Add(ctx, req, rsp)
	ctx.Write(rsp)
}

func (s *calcService) Sub(ctx context.Context, req *calcReq, rsp *calcRsp) {
	// The auto-generated handler passes the arpc Context.
	if _, ok := ctx.(*Context); !ok {
		panic("ctx is not an arpc Context")
	}
	rsp.Sum = req.A - req.B
}

func (s *calcService) Mul(ctx context.Context, req *calcReq, rsp *calcRsp) {
	rsp.Sum = req.A * req.B
}

func (s *calcService) MulBinding(ctx *Context) error { return nil }

func (s *calcService) Ping(ctx *Context) { ctx.Write("pong") }

func (s *calcService) Helper()                                              {}
func (s *calcService) Other(ctx context.Context, req calcReq, rsp *calcRsp) {}
func (s *calcService) unexported(ctx *Context)                              {}

func TestRegister(t *testing.T) {
	svc := &calcService{}
	var routes []string
	_, addr := startServer(t, func(h Handler) {
		if err := h.Register("Calc", svc); err != nil {
			t.Fatalf("Register: %v", err)
		}
		for route := range h.(*handler).routes {
			routes = append(routes, route)
		}
	})
	c := dialClient(t, addr, nil)

	// The "" route is the not-found handler.
	want := map[string]bool{"": true, "Calc.Add": true, "Calc.Sub": true, "Calc.Mul": true, "Calc.Ping": true}
	if len(routes) != len(want) {
		t.Fatalf("routes = %v", routes)
	}
	for _, r := range routes {
		if !want[r] {
			t.Fatalf("unexpected route %q in %v", r, routes)
		}
	}

	for route, sum := range map[string]int{"Calc.Add": 5, "Calc.Sub": 1, "Calc.Mul": 6} {
		var rsp calcRsp
		if err := c.Call(route, &calcReq{A: 3, B: 2}, &rsp, time.Second); err != nil || rsp.Sum != sum {
			t.Fatalf("%s = %v, %+v, want %v", route, err, rsp, sum)
		}
	}
	if svc.bindings != 1 {
		t.Fatalf("AddBinding called %v times, want 1", svc.bindings)
	}
	var pong string
	if err := c.Call("Calc.Ping", nil, &pong, time.Second); err != nil || pong != "pong" {
		t.Fatalf("Calc.Ping = %v, %q", err, pong)
	}

	// A request that cannot be bound gets an error response.
	var rsp calcRsp
	if err := c.Call("Calc.Sub", "not json", &rsp, time.Second); err == nil {
		t.Fatal("Calc.Sub with a bad request should fail")
	}
}

func TestRegister_Names(t *testing.T) {
	h := NewHandler()
	if err := h.Register("", &calcService{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Without a service name, the route is the method name.
	for _, route := range []string{"Add", "Sub", "Mul", "Ping"} {
		if _, ok := h.(*handler).routes[route]; !ok {
			t.Fatalf("route %q not registered", route)
		}
	}

	// Registering the same service name twice panics on the duplicates.
	mustPanic(t, "duplicate Register", func() { h.Register("", &calcService{}) })
}

type noMethodService struct{}

func (noMethodService) Foo()      {}
func (noMethodService) Bar(x int) {}

func TestRegister_Invalid(t *testing.T) {
	h := NewHandler()
	if err := h.Register("S", nil); err == nil {
		t.Fatal("Register(nil) should fail")
	}
	mustPanic(t, "Register without eligible methods", func() { h.Register("S", noMethodService{}) })
	mustPanic(t, "Register with too long a name", func() { h.Register(strings.Repeat("s", MaxMethodLen), &calcService{}) })
}

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

var (
	testServer           *Server
	testClientServerAddr = "localhost:11000"

	methodCallString   = "/callstring"
	methodCallBytes    = "/callbytes"
	methodCallStruct   = "/callstruct"
	methodCallWith     = "/callwith"
	methodCallAsync    = "/callasync"
	methodNotify       = "/notify"
	methodNotifyWith   = "/notifywith"
	methodCallError    = "/callerror"
	methodCallNotFound = "/notfound"
	methodInvalidLong  = `1234567890
						1234567890
						1234567890
						1234567890
						1234567890
						1234567890
						1234567890
						1234567890
						1234567890
						1234567890
						1234567890
						1234567890
						1234567890`

	invalidMethodErrString = fmt.Sprintf("invalid method length: %v, should <= %v", len(methodInvalidLong), MaxMethodLen)
)

type CoderTest int

func (ct *CoderTest) Encode(cli *Client, msg *Message) *Message {
	for i := 4; i < len(msg.Buffer); i++ {
		msg.Buffer[i] ^= 0xFF
	}
	return msg
}

func (ct *CoderTest) Decode(cli *Client, msg *Message) *Message {
	for i := 4; i < len(msg.Buffer); i++ {
		msg.Buffer[i] ^= 0xFF
	}
	return msg
}

type MessageTest struct {
	A int
	B string
}

func dialer() (net.Conn, error) {
	return net.DialTimeout("tcp", testClientServerAddr, time.Second)
}

func errDialer() (net.Conn, error) {
	return net.DialTimeout("tcp", "none", time.Second/1000)
}

func init() {
	HandleConnected(nil)
	HandleConnected(func(*Client) {})
	HandleConnected(func(*Client) {})
	HandleDisconnected(nil)
	HandleDisconnected(func(*Client) {})
	HandleDisconnected(func(*Client) {})
	UseCoder(new(CoderTest))
	Use(nil)
	Use(func(ctx *Context) {})
	Handle("-", func(ctx *Context) {})
	Use(func(ctx *Context) {})
	Use(func(ctx *Context) {})
	log.Println("AsyncResponse:", AsyncResponse())
	log.Println("BatchRecv:", BatchRecv())
	log.Println("BatchSend:", BatchSend())
	log.Println("AsyncResponse:", AsyncResponse())
	log.Println("RecvBufferSize:", RecvBufferSize())
	log.Println("SendQueueSize:", SendQueueSize())

}

func initServer() {
	requestCnt := 0
	testServer = NewServer()
	testServer.Handler.Handle(methodCallString, func(ctx *Context) {
		var src string
		ctx.Bind(&src)
		requestCnt++
		if requestCnt%2 == 0 {
			ctx.Write(src)
		} else {
			ctx.Write(&src)
		}
	}, true)
	testServer.Handler.Handle(methodCallBytes, func(ctx *Context) {
		var src []byte
		ctx.Bind(&src)
		requestCnt++
		if requestCnt%2 == 0 {
			ctx.Write(src)
		} else {
			ctx.Write(&src)
		}
	}, true)
	testServer.Handler.Handle(methodCallStruct, func(ctx *Context) {
		var src MessageTest
		ctx.Bind(&src)
		ctx.Write(&src)
	}, true)
	testServer.Handler.Handle(methodCallWith, func(ctx *Context) {
		ctx.WriteWithTimeout(ctx.Message.Data(), time.Second)
	}, true)
	testServer.Handler.Handle(methodCallAsync, func(ctx *Context) {
		ctx.Write(ctx.Message.Data())
	}, true)
	testServer.Handler.Handle(methodNotify, func(ctx *Context) {
		ctx.Bind(nil)
	}, false)
	testServer.Handler.Handle(methodNotifyWith, func(ctx *Context) {
		ctx.Bind(nil)
	}, false)
	testServer.Handler.Handle(methodCallError, func(ctx *Context) {
		ctx.Bind(nil)
		ctx.Error(ctx.Message.Data())
	}, false)
	go testServer.Run(testClientServerAddr)
	time.Sleep(time.Second / 10)
}

func TestClient_Get(t *testing.T) {
	c := &Client{}
	if v, ok := c.Get("key"); ok {
		t.Fatalf("Client.Get() error, returns %v, want nil", v)
	}
}

func TestClient_Set(t *testing.T) {
	key := "key"
	value := "value"

	c := &Client{}
	c.Set(key, nil)
	cv, ok := c.Get(key)
	if ok {
		t.Fatalf("Client.Get() failed: Get '%v', want nil", cv)
	}

	c.Set(key, value)
	cv, ok = c.Get(key)
	if !ok {
		t.Fatalf("Client.Get() failed: Get nil, want '%v'", value)
	}
	if cv != value {
		t.Fatalf("Client.Get() failed: Get '%v', want '%v'", cv, value)
	}
}

func TestClient_NewMessage(t *testing.T) {
	c := &Client{}
	for cmd := byte(1); cmd <= 3; cmd++ {
		method := fmt.Sprintf("method_%v", cmd)
		message := fmt.Sprintf("message_%v", cmd)
		msg := c.NewMessage(cmd, method, message)
		if msg == nil {
			t.Fatalf("Client.NewMessage() = nil")
		}
		if msg.Cmd() != cmd {
			t.Fatalf("Client.NewMessage() error, cmd is: %v, want: %v", msg.Cmd(), cmd)
		}
		if msg.Method() != method {
			t.Fatalf("Client.NewMessage() error, cmd is: %v, want: %v", msg.Method(), method)
		}
		if msg.Method() != method {
			t.Fatalf("Client.NewMessage() error, cmd is: %v, want: %v", string(msg.Data()), message)
		}
	}
}

func TestClient_ErrDial(t *testing.T) {
	initServer()
	defer testServer.Stop()

	_, err := NewClient(errDialer)
	if err == nil {
		t.Fatalf("NewClient with errDialer failed, returns nil error")
	}

	_, err = NewClientPoolFromDialers([]DialerFunc{dialer, dialer, errDialer, dialer, dialer})
	if err == nil {
		t.Fatalf("NewClientPoolFromDialers failed, returns nil error")
	}
}

func TestClient_Call(t *testing.T) {
	initServer()

	SetBatchRecv(false)
	SetBatchSend(false)

	c, err := NewClient(dialer)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	testClientCallMethodString(c, t)
	testClientCallMethodBytes(c, t)
	testClientCallMethodStruct(c, t)
	testClientCallError(c, t)
	testClientCallDisconnected(c, t)

	SetBatchRecv(true)
	SetBatchSend(true)
}

func testClientCallMethodString(c *Client, t *testing.T) {
	var (
		err error
		req = "hello"
		rsp = ""
	)
	if err = c.Call(methodCallString, req, &rsp, time.Second); err != nil {
		t.Fatalf("Client.Call() error = %v", err)
	} else if rsp != req {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.Call(methodCallString, &req, &rsp, -1); err != nil {
		t.Fatalf("Client.Call() error = %v", err)
	} else if rsp != req {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.Call(methodCallString, &req, nil, -1); err != nil {
		t.Fatalf("Client.Call() error = %v", err)
	}
}

func testClientCallMethodBytes(c *Client, t *testing.T) {
	var (
		err error
		req = []byte{1}
		rsp = []byte{}
	)
	if err = c.Call(methodCallBytes, req, &rsp, time.Second); err != nil {
		t.Fatalf("Client.Call() error = %v", err)
	} else if string(rsp) != string(req) {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.Call(methodCallBytes, &req, &rsp, -1); err != nil {
		t.Fatalf("Client.Call() error = %v", err)
	} else if string(rsp) != string(req) {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.Call(methodCallBytes, &req, nil, -1); err != nil {
		t.Fatalf("Client.Call() error = %v", err)
	}
}

func testClientCallMethodStruct(c *Client, t *testing.T) {
	var (
		err error
		req = MessageTest{A: 3, B: "4"}
		rsp = MessageTest{}
	)
	if err = c.Call(methodCallStruct, &req, &rsp, time.Second); err != nil {
		t.Fatalf("Client.Call() error = %v", err)
	} else if rsp.A != req.A || rsp.B != req.B {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.Call(methodCallStruct, &req, nil, time.Second); err != nil {
		t.Fatalf("Client.Call() error = %v", err)
	}
}

func testClientCallError(c *Client, t *testing.T) {
	var (
		err error
		req = "my error"
		rsp = ""
	)
	if err = c.Call(methodCallError, req, &rsp, time.Second); err == nil {
		t.Fatalf("Client.Call() error = nil, want '%v'", req)
	} else if err.Error() != req {
		t.Fatalf("Client.Call() error = '%v', want '%v'", err, req)
	}
	if rsp != "" {
		t.Fatalf("Client.Call() rsp = '%v', want ''", rsp)
	}

	if err = c.Call(methodCallString, "", nil, 0); err == nil {
		t.Fatalf("Client.Call() error is nil, want %v", ErrClientInvalidTimeoutZero)
	} else if err.Error() != ErrClientInvalidTimeoutZero.Error() {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", err.Error(), ErrClientInvalidTimeoutZero.Error())
	}

	if err = c.Call(methodCallNotFound, "", nil, -1); err == nil {
		t.Fatalf("Client.Call() error is nil, want %v", ErrMethodNotFound)
	} else if err.Error() != ErrMethodNotFound.Error() {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", err.Error(), ErrMethodNotFound.Error())
	}

	if err = c.Call(methodInvalidLong, "", nil, -1); err == nil {
		t.Fatalf("Client.Call() error is nil, want %v", invalidMethodErrString)
	} else if err.Error() != invalidMethodErrString {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", err.Error(), invalidMethodErrString)
	}
}

func testClientCallDisconnected(c *Client, t *testing.T) {
	var err error
	c.Stop()
	if err = c.Call(methodCallString, "", nil, -1); err == nil {
		t.Fatalf("Client.Call() error is nil, want %v", ErrClientStopped)
	} else if err.Error() != ErrClientStopped.Error() {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", err.Error(), ErrClientStopped.Error())
	}

	c.Restart()
	testServer.Stop()
	time.Sleep(time.Second / 10)
	if err = c.Call(methodCallString, "", nil, -1); err == nil {
		t.Fatalf("Client.Call() error is nil, want %v", ErrClientReconnecting)
	} else if err.Error() != ErrClientReconnecting.Error() {
		t.Fatalf("Client.Call() error, returns '%v', want '%v'", err.Error(), ErrClientReconnecting.Error())
	}
}

func TestClient_CallWith(t *testing.T) {
	initServer()

	c, err := NewClient(dialer)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	testClientCallWithMethodString(c, t)
	testClientCallWithMethodBytes(c, t)
	testClientCallWithMethodStruct(c, t)
	testClientCallWithError(c, t)
	testClientCallWithDisconnected(c, t)
}

func testClientCallWithMethodString(c *Client, t *testing.T) {
	var (
		err error
		req = "hello"
		rsp = ""
	)
	if err = c.CallWith(context.Background(), methodCallWith, req, &rsp); err != nil {
		t.Fatalf("Client.CallWith() error = %v", err)
	} else if rsp != req {
		t.Fatalf("Client.CallWith() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.CallWith(context.Background(), methodCallWith, &req, &rsp); err != nil {
		t.Fatalf("Client.CallWith() error = %v", err)
	} else if rsp != req {
		t.Fatalf("Client.CallWith() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.CallWith(context.Background(), methodCallWith, &req, nil); err != nil {
		t.Fatalf("Client.CallWith() error = %v", err)
	}
}

func testClientCallWithMethodBytes(c *Client, t *testing.T) {
	var (
		err error
		req = []byte{1}
		rsp = []byte{}
	)
	if err = c.CallWith(context.Background(), methodCallWith, req, &rsp); err != nil {
		t.Fatalf("Client.CallWith() error = %v", err)
	} else if string(rsp) != string(req) {
		t.Fatalf("Client.CallWith() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.CallWith(context.Background(), methodCallWith, &req, &rsp); err != nil {
		t.Fatalf("Client.CallWith() error = %v", err)
	} else if string(rsp) != string(req) {
		t.Fatalf("Client.CallWith() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.CallWith(context.Background(), methodCallWith, &req, nil); err != nil {
		t.Fatalf("Client.CallWith() error = %v", err)
	}
}

func testClientCallWithMethodStruct(c *Client, t *testing.T) {
	var (
		err error
		req = MessageTest{A: 3, B: "4"}
		rsp = MessageTest{}
	)
	if err = c.CallWith(context.Background(), methodCallWith, &req, &rsp); err != nil {
		t.Fatalf("Client.CallWith() error = %v", err)
	} else if rsp.A != req.A || rsp.B != req.B {
		t.Fatalf("Client.CallWith() error, returns '%v', want '%v'", rsp, req)
	}
	if err = c.CallWith(context.Background(), methodCallWith, &req, nil); err != nil {
		t.Fatalf("Client.CallWith() error = %v", err)
	}
}

func testClientCallWithError(c *Client, t *testing.T) {
	if err := c.CallWith(context.Background(), methodInvalidLong, "", nil); err == nil {
		t.Fatalf("Client.CallWith() error is nil, want %v", invalidMethodErrString)
	} else if err.Error() != invalidMethodErrString {
		t.Fatalf("Client.CallWith() error, returns '%v', want '%v'", err.Error(), invalidMethodErrString)
	}
}

func testClientCallWithDisconnected(c *Client, t *testing.T) {
	var err error
	c.Stop()
	if err = c.CallWith(context.Background(), methodCallWith, "", nil); err == nil {
		t.Fatalf("Client.CallWith() error is nil, want %v", ErrClientStopped)
	} else if err.Error() != ErrClientStopped.Error() {
		t.Fatalf("Client.CallWith() error, returns '%v', want '%v'", err.Error(), ErrClientStopped.Error())
	}

	c.Restart()
	testServer.Stop()
	time.Sleep(time.Second / 10)
	if err = c.CallWith(context.Background(), methodCallWith, "", nil); err == nil {
		t.Fatalf("Client.CallWith() error is nil, want %v", ErrClientReconnecting)
	} else if err.Error() != ErrClientReconnecting.Error() {
		t.Fatalf("Client.CallWith() error, returns '%v', want '%v'", err.Error(), ErrClientReconnecting.Error())
	}
}

func TestClient_CallAsync(t *testing.T) {
	initServer()

	c, err := NewClient(dialer)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	testClientCallAsyncMethodString(c, t)
	testClientCallAsyncMethodBytes(c, t)
	testClientCallAsyncMethodStruct(c, t)
	testClientCallAsyncError(c, t)
	testClientCallAsyncDisconnected(c, t)
}

func getAsyncHandler() (func(*Context), chan struct{}) {
	done := make(chan struct{}, 1)
	asyncHandler := func(*Context) {
		done <- struct{}{}
	}
	return asyncHandler, done
}

func testClientCallAsyncMethodString(c *Client, t *testing.T) {
	var (
		err error
		req = "hello"
	)
	asyncHandler, done := getAsyncHandler()
	if err = c.CallAsync(methodCallAsync, req, asyncHandler, time.Second); err != nil {
		t.Fatalf("Client.CallAsync() error = %v", err)
	}
	<-done
	if err = c.CallAsync(methodCallAsync, &req, asyncHandler, time.Second); err != nil {
		t.Fatalf("Client.CallAsync() error = %v", err)
	}
	<-done
	if err = c.CallAsync(methodCallAsync, &req, nil, time.Second); err != nil {
		t.Fatalf("Client.CallAsync() error = %v", err)
	}
}

func testClientCallAsyncMethodBytes(c *Client, t *testing.T) {
	var (
		err error
		req = []byte{1}
	)
	asyncHandler, done := getAsyncHandler()
	if err = c.CallAsync(methodCallAsync, req, asyncHandler, time.Second); err != nil {
		t.Fatalf("Client.CallAsync() error = %v", err)
	}
	<-done
	if err = c.CallAsync(methodCallAsync, &req, asyncHandler, time.Second); err != nil {
		t.Fatalf("Client.CallAsync() error = %v", err)
	}
	<-done
	if err = c.CallAsync(methodCallAsync, &req, nil, time.Second); err != nil {
		t.Fatalf("Client.CallAsync() error = %v", err)
	}
}

func testClientCallAsyncMethodStruct(c *Client, t *testing.T) {
	var (
		err error
		req = MessageTest{A: 3, B: "4"}
	)
	asyncHandler, done := getAsyncHandler()
	if err = c.CallAsync(methodCallAsync, &req, asyncHandler, time.Second); err != nil {
		t.Fatalf("Client.CallAsync() error = %v", err)
	}
	<-done
	if err = c.CallAsync(methodCallAsync, &req, asyncHandler, time.Second); err != nil {
		t.Fatalf("Client.CallAsync() error = %v", err)
	}
	<-done
}

func testClientCallAsyncError(c *Client, t *testing.T) {
	var err error
	asyncHandler, _ := getAsyncHandler()
	if err = c.CallAsync(methodCallAsync, "", asyncHandler, -1); err == nil {
		t.Fatalf("Client.CallAsync() error is nil, want %v", ErrClientInvalidTimeoutLessThanZero.Error())
	} else if err.Error() != ErrClientInvalidTimeoutLessThanZero.Error() {
		t.Fatalf("Client.CallAsync() error, returns '%v', want '%v'", err.Error(), ErrClientInvalidTimeoutLessThanZero.Error())
	}
	if err = c.CallAsync(methodCallAsync, "", nil, -1); err == nil {
		t.Fatalf("Client.CallAsync() error is nil, want %v", ErrClientInvalidTimeoutLessThanZero.Error())
	} else if err.Error() != ErrClientInvalidTimeoutLessThanZero.Error() {
		t.Fatalf("Client.CallAsync() error, returns '%v', want '%v'", err.Error(), ErrClientInvalidTimeoutLessThanZero.Error())
	}

	asyncHandler, _ = getAsyncHandler()
	if err = c.CallAsync(methodCallAsync, "", asyncHandler, 0); err == nil {
		t.Fatalf("Client.CallAsync() error is nil, want %v", ErrClientInvalidTimeoutZeroWithNonNilHandler.Error())
	} else if err.Error() != ErrClientInvalidTimeoutZeroWithNonNilHandler.Error() {
		t.Fatalf("Client.CallAsync() error, returns '%v', want '%v'", err.Error(), ErrClientInvalidTimeoutZeroWithNonNilHandler.Error())
	}

	invalidMethodErrString := fmt.Sprintf("invalid method length: %v, should <= %v", len(methodInvalidLong), MaxMethodLen)
	if err = c.CallAsync(methodInvalidLong, "", nil, time.Second); err == nil {
		t.Fatalf("Client.CallAsync() error is nil, want %v", invalidMethodErrString)
	} else if err.Error() != invalidMethodErrString {
		t.Fatalf("Client.CallAsync() error, returns '%v', want '%v'", err.Error(), invalidMethodErrString)
	}
}

func testClientCallAsyncDisconnected(c *Client, t *testing.T) {
	var err error
	c.Stop()
	if err = c.CallAsync(methodCallAsync, "", nil, time.Second); err == nil {
		t.Fatalf("Client.CallAsync() error is nil, want %v", ErrClientStopped)
	} else if err.Error() != ErrClientStopped.Error() {
		t.Fatalf("Client.CallAsync() error, returns '%v', want '%v'", err.Error(), ErrClientStopped.Error())
	}

	c.Restart()
	testServer.Stop()
	time.Sleep(time.Second / 10)
	if err = c.CallAsync(methodCallAsync, "", nil, time.Second); err == nil {
		t.Fatalf("Client.CallAsync() error is nil, want %v", ErrClientReconnecting)
	} else if err.Error() != ErrClientReconnecting.Error() {
		t.Fatalf("Client.CallAsync() error, returns '%v', want '%v'", err.Error(), ErrClientReconnecting.Error())
	}
}

func TestClient_Notify(t *testing.T) {
	initServer()

	c, err := NewClient(dialer)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	testClientNotifyMethodString(c, t)
	testClientNotifyMethodBytes(c, t)
	testClientNotifyMethodStruct(c, t)
	testClientNotifyError(c, t)
	testClientNotifyDisconnected(c, t)
}

func testClientNotifyMethodString(c *Client, t *testing.T) {
	var (
		err error
		req = "hello"
	)
	if err = c.Notify(methodNotify, req, time.Second); err != nil {
		t.Fatalf("Client.Notify() error = %v", err)
	}
	if err = c.Notify(methodNotify, &req, time.Second); err != nil {
		t.Fatalf("Client.Notify() error = %v", err)
	}
	if err = c.Notify(methodNotify, &req, time.Second); err != nil {
		t.Fatalf("Client.Notify() error = %v", err)
	}
}

func testClientNotifyMethodBytes(c *Client, t *testing.T) {
	var (
		err error
		req = []byte{1}
	)
	if err = c.Notify(methodNotify, req, time.Second); err != nil {
		t.Fatalf("Client.Notify() error = %v", err)
	}
	if err = c.Notify(methodNotify, &req, time.Second); err != nil {
		t.Fatalf("Client.Notify() error = %v", err)
	}
	if err = c.Notify(methodNotify, &req, time.Second); err != nil {
		t.Fatalf("Client.Notify() error = %v", err)
	}
}

func testClientNotifyMethodStruct(c *Client, t *testing.T) {
	var (
		err error
		req = MessageTest{A: 3, B: "4"}
	)
	if err = c.Notify(methodNotify, &req, time.Second); err != nil {
		t.Fatalf("Client.Notify() error = %v", err)
	}
	if err = c.Notify(methodNotify, &req, time.Second); err != nil {
		t.Fatalf("Client.Notify() error = %v", err)
	}
}

func testClientNotifyError(c *Client, t *testing.T) {
	var err error
	if err = c.Notify(methodNotify, "", -1); err == nil {
		t.Fatalf("Client.Notify() error is nil, want %v", ErrClientInvalidTimeoutLessThanZero.Error())
	} else if err.Error() != ErrClientInvalidTimeoutLessThanZero.Error() {
		t.Fatalf("Client.Notify() error, returns '%v', want '%v'", err.Error(), ErrClientInvalidTimeoutLessThanZero.Error())
	}
	if err = c.Notify(methodNotify, "", -1); err == nil {
		t.Fatalf("Client.Notify() error is nil, want %v", ErrClientInvalidTimeoutLessThanZero.Error())
	} else if err.Error() != ErrClientInvalidTimeoutLessThanZero.Error() {
		t.Fatalf("Client.Notify() error, returns '%v', want '%v'", err.Error(), ErrClientInvalidTimeoutLessThanZero.Error())
	}

	invalidMethodErrString := fmt.Sprintf("invalid method length: %v, should <= %v", len(methodInvalidLong), MaxMethodLen)
	if err = c.Notify(methodInvalidLong, "", time.Second); err == nil {
		t.Fatalf("Client.Notify() error is nil, want %v", invalidMethodErrString)
	} else if err.Error() != invalidMethodErrString {
		t.Fatalf("Client.Notify() error, returns '%v', want '%v'", err.Error(), invalidMethodErrString)
	}
}

func testClientNotifyDisconnected(c *Client, t *testing.T) {
	var err error
	c.Stop()
	if err = c.Notify(methodNotify, "", time.Second); err == nil {
		t.Fatalf("Client.Notify() error is nil, want %v", ErrClientStopped)
	} else if err.Error() != ErrClientStopped.Error() {
		t.Fatalf("Client.Notify() error, returns '%v', want '%v'", err.Error(), ErrClientStopped.Error())
	}

	c.Restart()
	testServer.Stop()
	time.Sleep(time.Second / 10)
	if err = c.Notify(methodNotify, "", time.Second); err == nil {
		t.Fatalf("Client.Notify() error is nil, want %v", ErrClientReconnecting)
	} else if err.Error() != ErrClientReconnecting.Error() {
		t.Fatalf("Client.Notify() error, returns '%v', want '%v'", err.Error(), ErrClientReconnecting.Error())
	}
}

func TestClient_NotifyWith(t *testing.T) {
	initServer()

	c, err := NewClient(dialer)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	testClientNotifyWithMethodString(c, t)
	testClientNotifyWithMethodBytes(c, t)
	testClientNotifyWithMethodStruct(c, t)
	testClientNotifyWithError(c, t)
	testClientNotifyWithDisconnected(c, t)
}

func testClientNotifyWithMethodString(c *Client, t *testing.T) {
	var (
		err error
		req = "hello"
	)
	if err = c.NotifyWith(context.Background(), methodNotifyWith, req); err != nil {
		t.Fatalf("Client.NotifyWith() error = %v", err)
	}
	if err = c.NotifyWith(context.Background(), methodNotifyWith, &req); err != nil {
		t.Fatalf("Client.NotifyWith() error = %v", err)
	}
	if err = c.NotifyWith(context.Background(), methodNotifyWith, &req); err != nil {
		t.Fatalf("Client.NotifyWith() error = %v", err)
	}
}

func testClientNotifyWithMethodBytes(c *Client, t *testing.T) {
	var (
		err error
		req = []byte{1}
	)
	if err = c.NotifyWith(context.Background(), methodNotifyWith, req); err != nil {
		t.Fatalf("Client.NotifyWith() error = %v", err)
	}
	if err = c.NotifyWith(context.Background(), methodNotifyWith, &req); err != nil {
		t.Fatalf("Client.NotifyWith() error = %v", err)
	}
	if err = c.NotifyWith(context.Background(), methodNotifyWith, &req); err != nil {
		t.Fatalf("Client.NotifyWith() error = %v", err)
	}
}

func testClientNotifyWithMethodStruct(c *Client, t *testing.T) {
	var (
		err error
		req = MessageTest{A: 3, B: "4"}
	)
	if err = c.NotifyWith(context.Background(), methodNotifyWith, &req); err != nil {
		t.Fatalf("Client.NotifyWith() error = %v", err)
	}
	if err = c.NotifyWith(context.Background(), methodNotifyWith, &req); err != nil {
		t.Fatalf("Client.NotifyWith() error = %v", err)
	}
}

func testClientNotifyWithError(c *Client, t *testing.T) {
	var err error
	invalidMethodErrString := fmt.Sprintf("invalid method length: %v, should <= %v", len(methodInvalidLong), MaxMethodLen)
	if err = c.NotifyWith(context.Background(), methodInvalidLong, ""); err == nil {
		t.Fatalf("Client.NotifyWith() error is nil, want %v", invalidMethodErrString)
	} else if err.Error() != invalidMethodErrString {
		t.Fatalf("Client.NotifyWith() error, returns '%v', want '%v'", err.Error(), invalidMethodErrString)
	}
}

func testClientNotifyWithDisconnected(c *Client, t *testing.T) {
	var err error
	c.Stop()
	if err = c.NotifyWith(context.Background(), methodNotifyWith, ""); err == nil {
		t.Fatalf("Client.NotifyWith() error is nil, want %v", ErrClientStopped)
	} else if err.Error() != ErrClientStopped.Error() {
		t.Fatalf("Client.NotifyWith() error, returns '%v', want '%v'", err.Error(), ErrClientStopped.Error())
	}

	c.Restart()
	testServer.Stop()
	time.Sleep(time.Second / 10)
	if err = c.NotifyWith(context.Background(), methodNotifyWith, ""); err == nil {
		t.Fatalf("Client.NotifyWith() error is nil, want %v", ErrClientReconnecting)
	} else if err.Error() != ErrClientReconnecting.Error() {
		t.Fatalf("Client.NotifyWith() error, returns '%v', want '%v'", err.Error(), ErrClientReconnecting.Error())
	}
}

func TestClient_PushMsg(t *testing.T) {
	initServer()

	c, err := NewClient(dialer)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	msg := c.NewMessage(CmdRequest, methodCallString, "hello")

	if err = c.PushMsg(msg, -1); err != nil {
		t.Fatalf("Client.PushMsg() error = %v", err)
	}
	if err = c.PushMsg(msg, 0); err != nil {
		t.Fatalf("Client.PushMsg() error = %v", err)
	}
	if err = c.PushMsg(msg, time.Second); err != nil {
		t.Fatalf("Client.PushMsg() error = %v", err)
	}

	c.Stop()
	if err = c.PushMsg(msg, 0); err == nil {
		t.Fatalf("Client.PushMsg() error is nil, want %v", ErrClientStopped)
	} else if err.Error() != ErrClientStopped.Error() {
		t.Fatalf("Client.PushMsg() error, returns '%v', want '%v'", err.Error(), ErrClientStopped.Error())
	}

	c.Restart()
	testServer.Stop()
	time.Sleep(time.Second / 10)
	if err = c.PushMsg(msg, 0); err == nil {
		t.Fatalf("Client.PushMsg() error is nil, want %v", ErrClientReconnecting)
	} else if err.Error() != ErrClientReconnecting.Error() {
		t.Fatalf("Client.PushMsg() error, returns '%v', want '%v'", err.Error(), ErrClientReconnecting.Error())
	}
}

func TestClientPool(t *testing.T) {
	initServer()
	testNewClientPool(t)
	testNewClientPoolFromDialers(t)
}

func testNewClientPool(t *testing.T) {
	poolSize := 5
	pool, err := NewClientPool(dialer, poolSize)
	if err != nil {
		t.Fatalf("NewClientPool failed: %v", err)
	}
	defer pool.Stop()

	if pool.Size() != poolSize {
		t.Fatalf("ClientPool.Size() returns %v, want:: %v", pool.Size(), poolSize)
	}
	for i := 0; i < poolSize; i++ {
		if pool.Handler() != pool.Get(0).Handler {
			t.Fatalf("ClientPool.Handler() != server.Handler")
		}
	}
	for i := 0; i < poolSize; i++ {
		req := "hello"
		rsp := ""
		if err = pool.Get(i).Call(methodCallString, req, &rsp, time.Second); err != nil {
			t.Fatalf("ClientPool.Get(%v).Call() error = '%v'", i, err)
		} else if rsp != req {
			t.Fatalf("ClientPool.Get(%v).Call() error, returns '%v', want '%v'", i, rsp, req)
		}
	}
	for i := 0; i < poolSize*2; i++ {
		req := "hello"
		rsp := ""
		if err = pool.Next().Call(methodCallString, req, &rsp, time.Second); err != nil {
			t.Fatalf("ClientPool.Next().Call() error = '%v'", err)
		} else if rsp != req {
			t.Fatalf("ClientPool.Next().Call() error, returns '%v', want '%v'", rsp, req)
		}
	}
}

func testNewClientPoolFromDialers(t *testing.T) {
	poolSize := 5
	dialers := make([]DialerFunc, poolSize)
	for i := 0; i < poolSize; i++ {
		dialers[i] = dialer
	}
	pool, err := NewClientPoolFromDialers(dialers)
	if err != nil {
		t.Fatalf("NewClientPoolFromDialers failed: %v", err)
	}
	defer pool.Stop()
	if pool.Size() != poolSize {
		t.Fatalf("ClientPool.Size() returns %v, want:: %v", pool.Size(), poolSize)
	}
	for i := 0; i < poolSize; i++ {
		if pool.Handler() != pool.Get(0).Handler {
			t.Fatalf("ClientPool.Handler() != server.Handler")
		}
	}
	for i := 0; i < poolSize; i++ {
		req := "hello"
		rsp := ""
		if err = pool.Get(i).Call(methodCallString, req, &rsp, time.Second); err != nil {
			t.Fatalf("ClientPool.Get(%v).Call() error = '%v'", i, err)
		} else if rsp != req {
			t.Fatalf("ClientPool.Get(%v).Call() error, returns '%v', want '%v'", i, rsp, req)
		}
	}
	for i := 0; i < poolSize*2; i++ {
		req := "hello"
		rsp := ""
		if err = pool.Next().Call(methodCallString, req, &rsp, time.Second); err != nil {
			t.Fatalf("ClientPool.Next().Call() error = '%v'", err)
		} else if rsp != req {
			t.Fatalf("ClientPool.Next().Call() error, returns '%v', want '%v'", rsp, req)
		}
	}

	testServer.Stop()
	time.Sleep(time.Second / 10)
}

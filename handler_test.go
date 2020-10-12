// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bytes"
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
	DefaultHandler.HandleOverstock(func(c *Client, m Message) {})
}

func Test_handler_OnOverstock(t *testing.T) {
	DefaultHandler.OnOverstock(nil, nil)
}

func Test_handler_HandleSessionMiss(t *testing.T) {
	DefaultHandler.HandleSessionMiss(func(c *Client, m Message) {})
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
	if got := DefaultHandler.RecvBufferSize(); got != 4096 {
		t.Errorf("handler.RecvBufferSize() = %v, want %v", got, 4096)
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

func Test_handler_Recv(t *testing.T) {
	c := &Client{}
	c.Head = Header(c.head[:])
	c.Reader = bytes.NewReader([]byte(NewMessage(CmdRequest, "hello", "hello", codec.DefaultCodec)))
	_, err := DefaultHandler.Recv(c)
	if err != nil {
		t.Errorf("handler.Recv() error = nil")
	}
}

func Test_handler_Send(t *testing.T) {
}

func Test_handler_SendN(t *testing.T) {
}

func Test_handler_OnMessage(t *testing.T) {

	done := make(chan int, 1)
	DefaultHandler.Handle("/onmessage", func(*Context) {
		done <- 1
	})
	DefaultHandler.OnMessage(nil, NewMessage(CmdRequest, "/onmessage", "hello", codec.DefaultCodec))
	select {
	case <-done:
	default:
		t.Errorf("OnMessage not handled")
	}

	c := newClientWithConn(&net.TCPConn{}, codec.DefaultCodec, DefaultHandler, nil)
	msg := NewMessage(CmdRequest, "/nohandler", "hello", codec.DefaultCodec)
	msg[headerIndexMethodLen] = 0
	DefaultHandler.OnMessage(c, msg)

	msg = NewMessage(CmdRequest, "/onmessage", "hello", codec.DefaultCodec)
	msg[headerIndexMethodLen] = 0
	DefaultHandler.OnMessage(c, msg)

	msg = NewMessage(CmdResponse, "/onmessage", "hello", codec.DefaultCodec)
	msg[headerIndexMethodLen] = 0
	DefaultHandler.OnMessage(c, msg)
}

func TestNewHandler(t *testing.T) {
	if got := NewHandler(); got == nil {
		t.Errorf("NewHandler() = nil")
	}
}

func TestSetHandler(t *testing.T) {
	h := NewHandler()
	SetHandler(h)
}

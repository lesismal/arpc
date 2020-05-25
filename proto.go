// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"unsafe"
)

const (
	// CmdNone is invalid
	CmdNone byte = 0

	// CmdRequest the other side should response to a request message
	CmdRequest byte = 1

	// CmdResponse the other side should not response to a request message
	CmdResponse byte = 2

	// CmdNotify the other side should not response to a request message
	CmdNotify byte = 3
)

var (
	refHeadLen   int  = 8
	refCntIndex  int  = 4
	refFlagByte  byte = 0x80
	refFlagIndex int  = 0
	// refMsgZeroFlag int32 = atomic.LoadInt32((*int32)(unsafe.Pointer(&([]byte{0, 0, 0, refFlagByte}[0]))))
)

const (
	headerIndexCmd          = 0
	headerIndexAsync        = 1
	headerIndexError        = 2
	headerIndexMethodLen    = 3
	headerIndexBodyLenBegin = 4
	headerIndexBodyLenEnd   = 8
	headerIndexSeqBegin     = 8
	headerIndexSeqEnd       = 16
)

const (
	// HeadLen defines rpc packet's head length
	HeadLen int = 16

	// MaxMethodLen limit
	MaxMethodLen int = 255

	// MaxBodyLen limit
	MaxBodyLen int = 1024*1024*64 - 16
)

// Header defines rpc head
type Header []byte

// BodyLen return length of message body
func (h Header) BodyLen() int {
	return int(binary.LittleEndian.Uint32(h[headerIndexBodyLenBegin:headerIndexBodyLenEnd]))
}

// message clones header with body length
func (h Header) message() (Message, error) {
	l := h.BodyLen()
	if l == 0 {
		return Message(h), nil
	}
	if l < 0 || l > MaxBodyLen {
		return nil, fmt.Errorf("invalid body length: %v", l)
	}
	m := Message(memGet(HeadLen + l))
	copy(m, h)

	return m, nil
}

// Message defines rpc packet
type Message []byte

// IsRef returns ref flag
func (m Message) IsRef() bool {
	return m[refFlagIndex] == refFlagByte
}

// Real returns real Message to send
func (m Message) Real() Message {
	if !m.IsRef() {
		return m
	}
	return Message(m[refHeadLen:])
}

// Cmd returns cmd
func (m Message) Cmd() byte {
	return m[headerIndexCmd]
}

// Async returns async flag value
func (m Message) Async() byte {
	return m[headerIndexAsync]
}

// IsAsync returns async flag
func (m Message) IsAsync() bool {
	return m[headerIndexAsync] == 1
}

// IsError returns error flag
func (m Message) IsError() bool {
	return m[headerIndexError] == 1
}

// Error returns error
func (m Message) Error() error {
	if !m.IsError() {
		return nil
	}
	return errors.New(bytesToStr(m[HeadLen:]))
}

// MethodLen returns method length
func (m Message) MethodLen() int {
	return int(m[headerIndexMethodLen])
}

// Method returns method
func (m Message) Method() string {
	return string(m[HeadLen : HeadLen+int(m[headerIndexMethodLen])])
}

// BodyLen return length of whole body[ method && body ]
func (m Message) BodyLen() int {
	return int(binary.LittleEndian.Uint32(m[headerIndexBodyLenBegin:headerIndexBodyLenEnd]))
}

// Seq returns sequence
func (m Message) Seq() uint64 {
	return binary.LittleEndian.Uint64(m[headerIndexSeqBegin:headerIndexSeqEnd])
}

// Body returns data after method
func (m Message) Body() []byte {
	length := HeadLen + m.MethodLen()
	return m[length:]
}

// Retain adds ref count
func (m Message) Retain() {
	if m.IsRef() {
		atomic.AddInt32((*int32)(unsafe.Pointer(&(m[refCntIndex]))), 1)
	}
}

// Release payback mem to MemPool
func (m Message) Release() {
	if !m.IsRef() {
		memPut(m)
	} else {
		if atomic.AddInt32((*int32)(unsafe.Pointer(&(m[refCntIndex]))), -1) == 0 {
			memPut(m)
		}
	}
}

func (m Message) cloneHead() Message {
	msg := Message(memGet(refHeadLen + HeadLen))
	msg[0], msg[1], msg[2], msg[3], msg[4], msg[5], msg[6], msg[7] = 0, 0, 0, 0, 0, 0, 0, 0
	msg[refFlagIndex] = refFlagByte
	copy(msg[refHeadLen:], m[:HeadLen])
	return msg
}

// NewRefMessage factory
func NewRefMessage(cmd byte, codec Codec, method string, v interface{}) Message {
	var (
		data    []byte
		msg     Message
		bodyLen int
	)

	data = valueToBytes(codec, v)

	bodyLen = len(method) + len(data)

	msg = Message(memGet(refHeadLen + HeadLen + bodyLen))
	msg[0], msg[1], msg[2], msg[3], msg[4], msg[5], msg[6], msg[7] = 0, 0, 0, 0, 0, 0, 0, 0
	msg[refFlagIndex] = refFlagByte
	binary.LittleEndian.PutUint32(msg[refHeadLen+headerIndexBodyLenBegin:refHeadLen+headerIndexBodyLenEnd], uint32(bodyLen))
	// binary.LittleEndian.PutUint64(msg[headerIndexSeqBegin:headerIndexSeqEnd], atomic.AddUint64(&c.seq, 1))

	msg[refHeadLen+headerIndexCmd] = cmd
	msg[refHeadLen+headerIndexMethodLen] = byte(len(method))
	copy(msg[refHeadLen+HeadLen:refHeadLen+HeadLen+len(method)], method)
	copy(msg[refHeadLen+HeadLen+len(method):], data)

	atomic.AddInt32((*int32)(unsafe.Pointer(&(msg[refCntIndex]))), 1)

	return msg
}

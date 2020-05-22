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
	// RPCCmdNone is invalid
	RPCCmdNone byte = 0
	// RPCCmdReq Call/Notify
	RPCCmdReq byte = 1
	// RPCCmdRsp normal response
	RPCCmdRsp byte = 2
	// RPCCmdErr error response
	RPCCmdErr byte = 3
)

var (
	refFlagByte    byte  = 0x80
	refMsgZeroFlag int32 = atomic.LoadInt32((*int32)(unsafe.Pointer(&([]byte{refFlagByte, 0, 0, 0}[0]))))
)

const (
	headerIndexCmd          = 0
	headerIndexAsync        = 1
	headerIndexMethodLen    = 2
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

// Payload returns real Message to send
func (m Message) Payload() Message {
	if m[0]&refFlagByte == 0 {
		return m
	}
	return Message(m[4:])
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
	return m[HeadLen+m.MethodLen():]
}

// Retain adds ref count
func (m Message) Retain() {
	if m[0]&refFlagByte != 0 {
		atomic.AddInt32((*int32)(unsafe.Pointer(&(m[0]))), 1)
	}
}

// Release payback mem to MemPool
func (m Message) Release() {
	if m[0]&refFlagByte == 0 {
		memPut(m)
	} else {
		if atomic.AddInt32((*int32)(unsafe.Pointer(&(m[0]))), -1) == refMsgZeroFlag {
			memPut(m)
		}
	}
}

// Parse head and body, return cmd && seq && isAsync && method && body && err
func (m Message) Parse() (byte, uint64, bool, string, []byte, error) {
	cmd := m.Cmd()
	switch cmd {
	case RPCCmdReq:
		ml := m.MethodLen()
		// have methond, return method and body
		if ml > 0 {
			if len(m) >= HeadLen+ml {
				return cmd, m.Seq(), m.IsAsync(), string(m[HeadLen : HeadLen+ml]), m[HeadLen+ml:], nil
			}
		} else {
			// no null method, return body
			return cmd, m.Seq(), m.IsAsync(), "", m[HeadLen:], nil
		}
	case RPCCmdRsp:
		return cmd, m.Seq(), m.IsAsync(), "", m[HeadLen:], nil
	case RPCCmdErr:
		return cmd, m.Seq(), m.IsAsync(), "", nil, errors.New(string(m[HeadLen]))
	default:
	}

	return RPCCmdNone, m.Seq(), m.IsAsync(), "", nil, ErrInvalidMessage
}

// NewRefMessage factory
func NewRefMessage(codec Codec, method string, v interface{}) Message {
	var (
		data    []byte
		msg     Message
		bodyLen int
	)

	data = valueToBytes(codec, v)

	bodyLen = len(method) + len(data)

	msg = Message(memGet(4 + HeadLen + bodyLen))
	msg[0], msg[1], msg[2], msg[3] = refFlagByte, 0, 0, 0
	binary.LittleEndian.PutUint32(msg[4+headerIndexBodyLenBegin:4+headerIndexBodyLenEnd], uint32(bodyLen))
	// binary.LittleEndian.PutUint64(msg[headerIndexSeqBegin:headerIndexSeqEnd], atomic.AddUint64(&c.seq, 1))

	msg[4+headerIndexCmd] = RPCCmdReq
	msg[4+headerIndexMethodLen] = byte(len(method))
	copy(msg[4+HeadLen:4+HeadLen+len(method)], method)
	copy(msg[4+HeadLen+len(method):], data)

	atomic.AddInt32((*int32)(unsafe.Pointer(&(msg[0]))), 1)

	return msg
}

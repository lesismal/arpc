// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/util"
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

const (
	// HeaderIndexBodyLenBegin .
	HeaderIndexBodyLenBegin = 0
	// HeaderIndexBodyLenEnd .
	HeaderIndexBodyLenEnd = 4
	// HeaderIndexSeqBegin .
	HeaderIndexSeqBegin = 4
	// HeaderIndexSeqEnd .
	HeaderIndexSeqEnd = 12
	// HeaderIndexCmd .
	HeaderIndexCmd = 12
	// HeaderIndexFlag .
	HeaderIndexFlag = 13
	// HeaderIndexReserved .
	HeaderIndexReserved = 14
	// HeaderIndexMethodLen .
	HeaderIndexMethodLen = 15

	// HeaderFlagMaskError .
	HeaderFlagMaskError byte = 0x01
	// HeaderFlagMaskAsync .
	HeaderFlagMaskAsync byte = 0x02
)

const (
	// HeadLen defines rpc packet's head length
	HeadLen int = 16

	// MaxMethodLen limit
	MaxMethodLen int = 127

	// MaxBodyLen limit
	MaxBodyLen int = 1024*1024*64 - 16
)

// Header defines rpc head
type Header []byte

// BodyLen return length of message body
func (h Header) BodyLen() int {
	return int(binary.LittleEndian.Uint32(h[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd]))
}

// message clones header with body length
func (h Header) message(handler Handler) (Message, error) {
	bodyLen := h.BodyLen()
	if bodyLen < 0 || bodyLen > MaxBodyLen {
		return nil, fmt.Errorf("invalid body length: %v", bodyLen)
	}

	m := Message(handler.GetBuffer(HeadLen + bodyLen))
	binary.LittleEndian.PutUint32(h[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(bodyLen))
	return m, nil
}

// Message defines rpc packet
type Message []byte

// Cmd returns cmd
func (m Message) Cmd() byte {
	return m[HeaderIndexCmd]
}

// IsAsync returns async flag
func (m Message) IsAsync() bool {
	return m[HeaderIndexFlag]&HeaderFlagMaskAsync > 0
}

// SetAsync sets async flag
func (m Message) SetAsync(isAsync bool) {
	if isAsync {
		m[HeaderIndexFlag] |= HeaderFlagMaskAsync
	} else {
		m[HeaderIndexFlag] &= ^HeaderFlagMaskAsync
	}
}

// IsError returns error flag
func (m Message) IsError() bool {
	return m[HeaderIndexFlag]&HeaderFlagMaskError > 0
}

// SetError sets error flag
func (m Message) SetError(isError bool) {
	if isError {
		m[HeaderIndexFlag] |= HeaderFlagMaskError
	} else {
		m[HeaderIndexFlag] &= ^HeaderFlagMaskError
	}
}

// Error returns error
func (m Message) Error() error {
	if !m.IsError() {
		return nil
	}
	return errors.New(util.BytesToStr(m[HeadLen+m.MethodLen():]))
}

// MethodLen returns method length
func (m Message) MethodLen() int {
	return int(m[HeaderIndexMethodLen])
}

// Method returns method
func (m Message) Method() string {
	return string(m[HeadLen : HeadLen+m.MethodLen()])
}

// BodyLen returns length of body[ method && body ]
func (m Message) BodyLen() int {
	return int(binary.LittleEndian.Uint32(m[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd]))
}

// SetBodyLen sets length of body[ method && body ]
func (m Message) SetBodyLen(l int) {
	binary.LittleEndian.PutUint32(m[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(l))
}

// Seq returns sequence
func (m Message) Seq() uint64 {
	return binary.LittleEndian.Uint64(m[HeaderIndexSeqBegin:HeaderIndexSeqEnd])
}

// Data returns data after method
func (m Message) Data() []byte {
	length := HeadLen + m.MethodLen()
	return m[length:]
}

// newMessage factory
func newMessage(cmd byte, method string, v interface{}, h Handler, codec codec.Codec) Message {
	var (
		data    []byte
		msg     Message
		bodyLen int
	)

	data = util.ValueToBytes(codec, v)
	bodyLen = len(method) + len(data)

	if h == nil {
		h = DefaultHandler
	}
	msg = Message(h.GetBuffer(HeadLen + bodyLen))
	msg[HeaderIndexCmd] = cmd
	msg[HeaderIndexMethodLen] = byte(len(method))
	binary.LittleEndian.PutUint32(msg[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(bodyLen))
	copy(msg[HeadLen:HeadLen+len(method)], method)
	copy(msg[HeadLen+len(method):], data)

	return msg
}

// MessageCoder .
type MessageCoder interface {
	Encode(Message) Message
	Decode(Message) Message
}

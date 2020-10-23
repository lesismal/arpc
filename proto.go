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
	// HeaderIndexReserved .
	HeaderIndexReserved = 4
	// HeaderIndexCmd .
	HeaderIndexCmd = 5
	// HeaderIndexFlag .
	HeaderIndexFlag = 6
	// HeaderIndexMethodLen .
	HeaderIndexMethodLen = 7
	// HeaderIndexSeqBegin .
	HeaderIndexSeqBegin = 8
	// HeaderIndexSeqEnd .
	HeaderIndexSeqEnd = 16
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
func (h Header) message(handler Handler) (*Message, error) {
	bodyLen := h.BodyLen()
	if bodyLen < 0 || bodyLen > MaxBodyLen {
		return nil, fmt.Errorf("invalid body length: %v", bodyLen)
	}

	m := &Message{Buffer: handler.GetBuffer(HeadLen + bodyLen)}
	binary.LittleEndian.PutUint32(m.Buffer[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(bodyLen))
	return m, nil
}

// Message defines rpc packet
type Message struct {
	Buffer []byte
	Values map[string]interface{}
}

// Len returns total length of buffer
func (m *Message) Len() int {
	return len(m.Buffer)
}

// Cmd returns cmd
func (m *Message) Cmd() byte {
	return m.Buffer[HeaderIndexCmd]
}

// SetCmd sets cmd
func (m *Message) SetCmd(cmd byte) {
	m.Buffer[HeaderIndexCmd] = cmd
}

// IsError returns error flag
func (m *Message) IsError() bool {
	return m.Buffer[HeaderIndexFlag]&HeaderFlagMaskError > 0
}

// SetError sets error flag
func (m *Message) SetError(isError bool) {
	if isError {
		m.Buffer[HeaderIndexFlag] |= HeaderFlagMaskError
	} else {
		m.Buffer[HeaderIndexFlag] &= ^HeaderFlagMaskError
	}
}

// Error returns error
func (m *Message) Error() error {
	if !m.IsError() {
		return nil
	}
	return errors.New(util.BytesToStr(m.Buffer[HeadLen+m.MethodLen():]))
}

// IsAsync returns async flag
func (m *Message) IsAsync() bool {
	return m.Buffer[HeaderIndexFlag]&HeaderFlagMaskAsync > 0
}

// SetAsync sets async flag
func (m *Message) SetAsync(isAsync bool) {
	if isAsync {
		m.Buffer[HeaderIndexFlag] |= HeaderFlagMaskAsync
	} else {
		m.Buffer[HeaderIndexFlag] &= ^HeaderFlagMaskAsync
	}
}

// SetFlagBit sets flag bit with value by index
func (m *Message) SetFlagBit(index int, value bool) error {
	switch index {
	case 0, 1, 2, 3, 4, 5, 6, 7:
		if value {
			m.Buffer[HeaderIndexReserved] |= (0x1 << index)
		} else {
			m.Buffer[HeaderIndexReserved] &= (^(0x1 << index))
		}
		return nil
	// case 8, 9:
	// 	if value {
	// 		m.Buffer[HeaderIndexFlag] |= (0x1 << (index - 2))
	// 	} else {
	// 		m.Buffer[HeaderIndexFlag] &= (^(0x1 << (index - 2)))
	// 	}
	// 	return nil
	default:
		break
	}
	return ErrInvalidFlagBitIndex
}

// IsFlagBitSet returns flag bit value
func (m *Message) IsFlagBitSet(index int) bool {
	switch index {
	case 0, 1, 2, 3, 4, 5, 6, 7:
		return (m.Buffer[HeaderIndexReserved] & (0x1 << index)) != 0
	// case 8, 9:
	// 	return (m.Buffer[HeaderIndexFlag] & (0x1 << (index - 2))) != 0
	default:
		break
	}
	return false
}

// MethodLen returns method length
func (m *Message) MethodLen() int {
	return int(m.Buffer[HeaderIndexMethodLen])
}

// SetMethodLen sets method length
func (m *Message) SetMethodLen(l int) {
	m.Buffer[HeaderIndexMethodLen] = byte(l)
}

// Method returns method
func (m *Message) Method() string {
	return string(m.Buffer[HeadLen : HeadLen+m.MethodLen()])
}

func (m *Message) method() string {
	return util.BytesToStr(m.Buffer[HeadLen : HeadLen+m.MethodLen()])
}

// BodyLen returns length of body[ method && body ]
func (m *Message) BodyLen() int {
	return int(binary.LittleEndian.Uint32(m.Buffer[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd]))
}

// SetBodyLen sets length of body[ method && body ]
func (m *Message) SetBodyLen(l int) {
	binary.LittleEndian.PutUint32(m.Buffer[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(l))
}

// Seq returns sequence
func (m *Message) Seq() uint64 {
	return binary.LittleEndian.Uint64(m.Buffer[HeaderIndexSeqBegin:HeaderIndexSeqEnd])
}

// SetSeq sets sequence
func (m *Message) SetSeq(seq uint64) {
	binary.LittleEndian.PutUint64(m.Buffer[HeaderIndexSeqBegin:HeaderIndexSeqEnd], seq)
}

// Data returns data after method
func (m *Message) Data() []byte {
	length := HeadLen + m.MethodLen()
	return m.Buffer[length:]
}

// Get returns value for key
func (m *Message) Get(key string) (interface{}, bool) {
	if len(m.Values) == 0 {
		return nil, false
	}
	value, ok := m.Values[key]
	return value, ok
}

// Set sets key-value pair
func (m *Message) Set(key string, value interface{}) {
	if value == nil {
		return
	}
	if m.Values == nil {
		m.Values = map[string]interface{}{}
	}
	m.Values[key] = value
}

// newMessage factory
func newMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, h Handler, codec codec.Codec, values map[string]interface{}) *Message {
	var (
		data    []byte
		msg     *Message
		bodyLen int
	)

	data = util.ValueToBytes(codec, v)
	bodyLen = len(method) + len(data)

	if h == nil {
		h = DefaultHandler
	}

	msg = &Message{Buffer: h.GetBuffer(HeadLen + bodyLen), Values: values}
	msg.SetCmd(cmd)
	msg.SetError(isError)
	msg.SetAsync(isAsync)
	msg.SetMethodLen(len(method))
	msg.SetBodyLen(bodyLen)
	msg.SetSeq(seq)
	copy(msg.Buffer[HeadLen:HeadLen+len(method)], method)
	copy(msg.Buffer[HeadLen+len(method):], data)

	return msg
}

func checkMethod(method string) error {
	ml := len(method)
	if ml == 0 || ml > MaxMethodLen {
		return fmt.Errorf("invalid method length: %v, should <= %v", ml, MaxMethodLen)
	}
	return nil
}

// MessageCoder .
type MessageCoder interface {
	// Encode wrap message before send to client
	Encode(*Client, *Message) *Message
	// Decode unwrap message between recv and handle
	Decode(*Client, *Message) *Message
}

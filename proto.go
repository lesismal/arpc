// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

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
	// HeadLen represents Message head length.
	HeadLen int = 16

	// MaxMethodLen limits Message method length.
	MaxMethodLen int = 127

	// DefaultMaxBodyLen limits Message body length.
	DefaultMaxBodyLen int = 1024*1024*64 - 16
)

// Header defines Message head
type Header []byte

// BodyLen returns Message body length.
func (h Header) BodyLen() int {
	return int(binary.LittleEndian.Uint32(h[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd]))
}

// message creates a Message by body length.
func (h Header) message(handler Handler) (*Message, error) {
	bodyLen := h.BodyLen()
	if bodyLen < 0 || bodyLen > handler.MaxBodyLen() {
		return nil, fmt.Errorf("invalid body length: %v", bodyLen)
	}

	// msg := &Message{Buffer: handler.Malloc(HeadLen + bodyLen)}
	msg := messagePool.Get().(*Message)
	msg.handler = handler
	msg.Buffer = handler.Malloc(HeadLen + bodyLen)

	binary.LittleEndian.PutUint32(msg.Buffer[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(bodyLen))
	return msg, nil
}

var (
	messagePool = sync.Pool{
		New: func() interface{} {
			return &Message{}
		},
	}

	emptyMessage = Message{}
)

// Message represents an arpc Message.
type Message struct {
	// 64-aligned on 32-bit
	ref int32

	Buffer []byte

	handler Handler
	values  map[interface{}]interface{}
}

// Retain increment the reference count and returns the current value.
func (m *Message) Retain() int32 {
	return atomic.AddInt32(&m.ref, 1)
}

// Release decrement the reference count and returns the current value.
func (m *Message) Release() int32 {
	n := atomic.AddInt32(&m.ref, -1)
	if n == -1 {
		if m.handler != nil {
			m.handler.Free(m.Buffer)
		}
		*m = emptyMessage
		messagePool.Put(m)
	}
	return n
}

// ResetAttrs resets reserved/cmd/flag/methodLen to 0.
func (m *Message) ResetAttrs() {
	binary.LittleEndian.PutUint32(m.Buffer[HeaderIndexBodyLenEnd:HeaderIndexSeqBegin], 0)
}

// Payback put Message to the pool.
func (m *Message) Payback() {
	*m = emptyMessage
	messagePool.Put(m)
}

// Len returns total length of buffer.
func (m *Message) Len() int {
	return len(m.Buffer)
}

// Cmd returns cmd.
func (m *Message) Cmd() byte {
	return m.Buffer[HeaderIndexCmd]
}

// SetCmd sets cmd.
func (m *Message) SetCmd(cmd byte) {
	m.Buffer[HeaderIndexCmd] = cmd
}

// IsError returns error flag.
func (m *Message) IsError() bool {
	return m.Buffer[HeaderIndexFlag]&HeaderFlagMaskError > 0
}

// SetError sets error flag.
func (m *Message) SetError(isError bool) {
	if isError {
		m.Buffer[HeaderIndexFlag] |= HeaderFlagMaskError
	} else {
		m.Buffer[HeaderIndexFlag] &= ^HeaderFlagMaskError
	}
}

// Error returns error.
func (m *Message) Error() error {
	if !m.IsError() {
		return nil
	}
	return errors.New(string(m.Buffer[HeadLen+m.MethodLen():]))
}

// IsAsync returns async flag.
func (m *Message) IsAsync() bool {
	return m.Buffer[HeaderIndexFlag]&HeaderFlagMaskAsync > 0
}

// SetAsync sets async flag.
func (m *Message) SetAsync(isAsync bool) {
	if isAsync {
		m.Buffer[HeaderIndexFlag] |= HeaderFlagMaskAsync
	} else {
		m.Buffer[HeaderIndexFlag] &= ^HeaderFlagMaskAsync
	}
}

// Values returns values.
func (m *Message) Values() map[interface{}]interface{} {
	return m.values
}

// SetFlagBit sets flag bit value by index.
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

// IsFlagBitSet returns flag bit value.
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

// MethodLen returns method length.
func (m *Message) MethodLen() int {
	return int(m.Buffer[HeaderIndexMethodLen])
}

// SetMethodLen sets method length.
func (m *Message) SetMethodLen(l int) {
	m.Buffer[HeaderIndexMethodLen] = byte(l)
}

// Method returns method.
func (m *Message) Method() string {
	return string(m.Buffer[HeadLen : HeadLen+m.MethodLen()])
}

func (m *Message) method() string {
	return util.BytesToStr(m.Buffer[HeadLen : HeadLen+m.MethodLen()])
}

// BodyLen returns body length.
func (m *Message) BodyLen() int {
	return int(binary.LittleEndian.Uint32(m.Buffer[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd]))
}

// SetBodyLen sets body length.
func (m *Message) SetBodyLen(l int) {
	binary.LittleEndian.PutUint32(m.Buffer[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(l))
}

// Seq returns sequence number.
func (m *Message) Seq() uint64 {
	return binary.LittleEndian.Uint64(m.Buffer[HeaderIndexSeqBegin:HeaderIndexSeqEnd])
}

// SetSeq sets sequence number.
func (m *Message) SetSeq(seq uint64) {
	binary.LittleEndian.PutUint64(m.Buffer[HeaderIndexSeqBegin:HeaderIndexSeqEnd], seq)
}

// Data returns payload data after method.
func (m *Message) Data() []byte {
	length := HeadLen + m.MethodLen()
	return m.Buffer[length:]
}

// Get returns value for key.
func (m *Message) Get(key interface{}) (interface{}, bool) {
	if len(m.values) == 0 {
		return nil, false
	}
	value, ok := m.values[key]
	return value, ok
}

// Set sets key-value pair.
func (m *Message) Set(key interface{}, value interface{}) {
	if key == nil || value == nil {
		return
	}
	if m.values == nil {
		m.values = map[interface{}]interface{}{}
	}
	m.values[key] = value
}

// NewMessage creates a Message.
func NewMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, h Handler, codec codec.Codec, values map[interface{}]interface{}) *Message {
	return newMessage(cmd, method, v, false, false, seq, h, codec, values)
}

// newMessage creates a Message.
func newMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, h Handler, codec codec.Codec, values map[interface{}]interface{}) *Message {
	var (
		data    []byte
		bodyLen int
		msg     *Message
	)

	data = util.ValueToBytes(codec, v)
	bodyLen = len(method) + len(data)

	if h == nil {
		h = DefaultHandler
	}

	// msg = &Message{Buffer: h.Malloc(HeadLen + bodyLen), values: values}
	msg = messagePool.Get().(*Message)
	msg.values = values
	msg.handler = h
	msg.Buffer = h.Malloc(HeadLen + bodyLen)

	msg.ResetAttrs()
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

// MessageCoder defines Message coding middleware interface.
type MessageCoder interface {
	// Encode wrap message before send to client
	Encode(*Client, *Message) *Message
	// Decode unwrap message between recv and handle
	Decode(*Client, *Message) *Message
}

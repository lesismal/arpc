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

// Message commands, stored in the low 6 bits of the Cmd byte of the header.
const (
	// CmdNone is invalid.
	CmdNone byte = 0

	// CmdRequest is a request that expects a response.
	CmdRequest byte = 1

	// CmdResponse is a response to a request.
	CmdResponse byte = 2

	// CmdNotify is a one-way message that expects no response.
	CmdNotify byte = 3

	// CmdPing is a heartbeat; the receiver replies with CmdPong.
	CmdPing byte = 4

	// CmdPong is the reply to CmdPing.
	CmdPong byte = 5

	// CmdStream is a Stream message.
	CmdStream byte = 6
)

// Header layout (HeadLen bytes, little endian), followed by the method name and
// then the payload data:
//
//	[0, 4)  body length: len(method) + len(data)
//	[4]     reserved byte for user flag bits, see Message.SetFlagBit
//	[5]     cmd in the low 6 bits, plus the stream EOF (bit 6) and local (bit 7) bits
//	[6]     flags: error, async
//	[7]     method length
//	[8, 16) sequence number
const (
	// HeaderIndexBodyLenBegin is where the body length begins.
	HeaderIndexBodyLenBegin = 0
	// HeaderIndexBodyLenEnd is where the body length ends.
	HeaderIndexBodyLenEnd = 4
	// HeaderIndexReserved is the index of the reserved byte.
	HeaderIndexReserved = 4
	// HeaderIndexCmd is the index of the cmd byte.
	HeaderIndexCmd = 5
	// HeaderIndexFlag is the index of the flag byte.
	HeaderIndexFlag = 6
	// HeaderIndexMethodLen is the index of the method length byte.
	HeaderIndexMethodLen = 7
	// HeaderIndexSeqBegin is where the sequence number begins.
	HeaderIndexSeqBegin = 8
	// HeaderIndexSeqEnd is where the sequence number ends.
	HeaderIndexSeqEnd = 16
	// HeaderFlagMaskError marks an error response.
	HeaderFlagMaskError byte = 0x01
	// HeaderFlagMaskAsync marks a message of an async call.
	HeaderFlagMaskAsync byte = 0x02

	// HeaderStreamLocalBitIndex is the bit index, in the cmd byte, of the flag
	// set on messages sent by the side that created the Stream.
	HeaderStreamLocalBitIndex = 7
	// HeaderStreamEOFBitIndex is the bit index, in the cmd byte, of the flag
	// set on the last message of a Stream's send half.
	HeaderStreamEOFBitIndex = 6
	// HeaderStreamLocalBit is the mask of the stream local bit.
	HeaderStreamLocalBit = byte(0x1) << HeaderStreamLocalBitIndex
	// HeaderStreamEOFBit is the mask of the stream EOF bit.
	HeaderStreamEOFBit = byte(0x1) << HeaderStreamEOFBitIndex
	// HeaderStreamFlagBitMask is the mask of all stream bits in the cmd byte.
	HeaderStreamFlagBitMask = HeaderStreamLocalBit | HeaderStreamEOFBit
	// HeaderCmdBitMask is the mask of the cmd bits in the cmd byte.
	HeaderCmdBitMask = ^HeaderStreamFlagBitMask
)

const (
	// HeadLen is the length of the Message header.
	HeadLen int = 16

	// MaxMethodLen is the max length of a method name.
	MaxMethodLen int = 127

	// DefaultMaxBodyLen is the default max body length (method + data) of a
	// received Message, see Handler.SetMaxBodyLen.
	DefaultMaxBodyLen int = 1024*1024*64 - 16
)

var (
	// PingMessage is the shared heartbeat message.
	PingMessage = newMessage(CmdPing, "", nil, false, false, 0, nil, nil, nil)

	// PongMessage is the shared reply to PingMessage.
	PongMessage = newMessage(CmdPong, "", nil, false, false, 0, nil, nil, nil)
)

// Header is the first HeaderIndexBodyLenEnd bytes of a Message read from the
// conn, i.e. the body length.
type Header []byte

// BodyLen returns the body length.
func (h Header) BodyLen() int {
	return int(binary.LittleEndian.Uint32(h[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd]))
}

// message allocates a Message large enough for the header and body, with the
// body length filled in. It fails if the body length exceeds
// Handler.MaxBodyLen.
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

// Message is an arpc message: the header, the method name and the payload
// data, laid out contiguously in Buffer.
//
// Messages are pooled and reference counted: the count starts at 0, Retain
// increments it and Release decrements it; the Message is freed when it drops
// below 0, so each Retain needs one extra Release.
type Message struct {
	// ref is the first field to keep it 64-bit aligned on 32-bit platforms.
	ref int32

	Buffer []byte

	// body is the payload data kept apart from Buffer, used only by the writev
	// path (see Client.pushWritev). When it is non-nil, Buffer holds only the
	// header and method, and both are written together with writev without
	// being concatenated. Otherwise it is nil and Buffer holds everything.
	body []byte
	// bodyPooled reports whether body was allocated by Handler.Malloc and must
	// be freed on Release. It is false when body belongs to the caller or the
	// codec.
	bodyPooled bool

	handler Handler
	values  map[interface{}]interface{}
}

// Retain increments the reference count and returns the new value.
func (m *Message) Retain() int32 {
	return atomic.AddInt32(&m.ref, 1)
}

// Release decrements the reference count and returns the new value. When it
// reaches -1, the buffers are freed by Handler.Free and the Message goes back
// to the pool.
func (m *Message) Release() int32 {
	n := atomic.AddInt32(&m.ref, -1)
	if n == -1 {
		if m.handler != nil {
			m.handler.Free(m.Buffer)
			if m.bodyPooled && m.body != nil {
				m.handler.Free(m.body)
			}
		}
		*m = emptyMessage
		messagePool.Put(m)
	}
	return n
}

// ResetAttrs zeroes the reserved, cmd, flag and method length bytes.
func (m *Message) ResetAttrs() {
	binary.LittleEndian.PutUint32(m.Buffer[HeaderIndexBodyLenEnd:HeaderIndexSeqBegin], 0)
}

// Payback puts the Message back to the pool without freeing its buffers,
// regardless of the reference count.
func (m *Message) Payback() {
	*m = emptyMessage
	messagePool.Put(m)
}

// Len returns the length of Buffer.
func (m *Message) Len() int {
	return len(m.Buffer)
}

// Cmd returns the cmd, without the stream bits.
func (m *Message) Cmd() byte {
	return m.Buffer[HeaderIndexCmd] & HeaderCmdBitMask
}

// SetCmd sets the cmd, keeping the stream bits.
func (m *Message) SetCmd(cmd byte) {
	m.Buffer[HeaderIndexCmd] = (m.Buffer[HeaderIndexCmd] & HeaderStreamFlagBitMask) | cmd
}

// // IsStream represents whether it's a stream message.
// func (m *Message) IsStream() bool {
// 	return m.Buffer[HeaderIndexCmd]&HeaderStreamBit > 0
// }

// // SetStream sets the flag for a stream message.
// func (m *Message) SetStream(isStream bool) {
// 	if isStream {
// 		m.Buffer[HeaderIndexCmd] |= HeaderStreamBit
// 	} else {
// 		m.Buffer[HeaderIndexCmd] &= (^HeaderStreamBit)
// 	}
// }

// IsStreamLocal reports whether the stream local bit is set, i.e. the message
// was sent by the side that created the Stream.
func (m *Message) IsStreamLocal() bool {
	return m.Buffer[HeaderIndexCmd]&HeaderStreamLocalBit > 0
}

// SetStreamLocal sets or clears the stream local bit.
func (m *Message) SetStreamLocal(local bool) {
	if local {
		m.Buffer[HeaderIndexCmd] |= HeaderStreamLocalBit
	} else {
		m.Buffer[HeaderIndexCmd] &= (^HeaderStreamLocalBit)
	}
}

// IsStreamEOF reports whether this is the last message from the sender's send
// half of the Stream.
func (m *Message) IsStreamEOF() bool {
	return m.Buffer[HeaderIndexCmd]&HeaderStreamEOFBit > 0
}

// SetStreamEOF sets or clears the stream EOF bit.
func (m *Message) SetStreamEOF(eof bool) {
	if eof {
		m.Buffer[HeaderIndexCmd] |= HeaderStreamEOFBit
	} else {
		m.Buffer[HeaderIndexCmd] &= (^HeaderStreamEOFBit)
	}
}

// IsError reports whether the error flag is set.
func (m *Message) IsError() bool {
	return m.Buffer[HeaderIndexFlag]&HeaderFlagMaskError > 0
}

// SetError sets or clears the error flag.
func (m *Message) SetError(isError bool) {
	if isError {
		m.Buffer[HeaderIndexFlag] |= HeaderFlagMaskError
	} else {
		m.Buffer[HeaderIndexFlag] &= ^HeaderFlagMaskError
	}
}

// Error returns the data as an error if the error flag is set, otherwise nil.
func (m *Message) Error() error {
	if !m.IsError() {
		return nil
	}
	return errors.New(string(m.Buffer[HeadLen+m.MethodLen():]))
}

// IsAsync reports whether the async flag is set.
func (m *Message) IsAsync() bool {
	return m.Buffer[HeaderIndexFlag]&HeaderFlagMaskAsync > 0
}

// SetAsync sets or clears the async flag.
func (m *Message) SetAsync(isAsync bool) {
	if isAsync {
		m.Buffer[HeaderIndexFlag] |= HeaderFlagMaskAsync
	} else {
		m.Buffer[HeaderIndexFlag] &= ^HeaderFlagMaskAsync
	}
}

// Values returns the key-value pairs attached to the Message. They are local
// and never sent over the wire.
func (m *Message) Values() map[interface{}]interface{} {
	return m.values
}

// SetFlagBit sets or clears bit index, 0-7, of the reserved byte, which is
// free for users. It returns ErrInvalidFlagBitIndex for other indexes.
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

// IsFlagBitSet reports whether bit index of the reserved byte is set. It
// returns false for an index out of 0-7.
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

// MethodLen returns the method length.
func (m *Message) MethodLen() int {
	return int(m.Buffer[HeaderIndexMethodLen])
}

// SetMethodLen sets the method length.
func (m *Message) SetMethodLen(l int) {
	m.Buffer[HeaderIndexMethodLen] = byte(l)
}

// Method returns a copy of the method name.
func (m *Message) Method() string {
	return string(m.Buffer[HeadLen : HeadLen+m.MethodLen()])
}

// method returns the method name without copying; it is only valid until the
// Message is released.
func (m *Message) method() string {
	return util.BytesToStr(m.Buffer[HeadLen : HeadLen+m.MethodLen()])
}

// BodyLen returns the body length (method + data) in the header.
func (m *Message) BodyLen() int {
	return int(binary.LittleEndian.Uint32(m.Buffer[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd]))
}

// SetBodyLen sets the body length in the header.
func (m *Message) SetBodyLen(l int) {
	binary.LittleEndian.PutUint32(m.Buffer[HeaderIndexBodyLenBegin:HeaderIndexBodyLenEnd], uint32(l))
}

// Seq returns the sequence number.
func (m *Message) Seq() uint64 {
	return binary.LittleEndian.Uint64(m.Buffer[HeaderIndexSeqBegin:HeaderIndexSeqEnd])
}

// SetSeq sets the sequence number.
func (m *Message) SetSeq(seq uint64) {
	binary.LittleEndian.PutUint64(m.Buffer[HeaderIndexSeqBegin:HeaderIndexSeqEnd], seq)
}

// Data returns the payload data after the method name, without copying.
func (m *Message) Data() []byte {
	length := HeadLen + m.MethodLen()
	return m.Buffer[length:]
}

// Get returns the value stored for key.
func (m *Message) Get(key interface{}) (interface{}, bool) {
	if len(m.values) == 0 {
		return nil, false
	}
	value, ok := m.values[key]
	return value, ok
}

// Set stores a key-value pair. It does nothing if key or value is nil.
func (m *Message) Set(key interface{}, value interface{}) {
	if key == nil || value == nil {
		return
	}
	if m.values == nil {
		m.values = map[interface{}]interface{}{}
	}
	m.values[key] = value
}

// NewMessage creates a Message from the pool, with v converted by
// util.ValueToBytes. A nil h means DefaultHandler.
//
// Note: isError and isAsync are ignored, both flags are left unset; use
// SetError and SetAsync instead.
func NewMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, h Handler, codec codec.Codec, values map[interface{}]interface{}) *Message {
	return newMessage(cmd, method, v, false, false, seq, h, codec, values)
}

// newMessage creates a Message from the pool with v encoded into a single
// contiguous Buffer. A nil h means DefaultHandler.
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

// newWritevMessage creates a Message for the writev path.
//
// Unlike newMessage, it does not copy the data into Buffer: Buffer holds only
// the header and method, and body holds the data. The header's body length
// still covers method + data, so the wire format is the same.
//
// Data from a []byte or *[]byte is copied into a Handler.Malloc buffer, since
// the caller may modify it before the async write completes.
func newWritevMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, h Handler, codec codec.Codec, values map[interface{}]interface{}) *Message {
	data, owned := util.ValueToBytesOwned(codec, v)
	bodyLen := len(method) + len(data)

	if h == nil {
		h = DefaultHandler
	}

	msg := messagePool.Get().(*Message)
	msg.values = values
	msg.handler = h
	msg.Buffer = h.Malloc(HeadLen + len(method))

	msg.ResetAttrs()
	msg.SetCmd(cmd)
	msg.SetError(isError)
	msg.SetAsync(isAsync)
	msg.SetMethodLen(len(method))
	msg.SetBodyLen(bodyLen)
	msg.SetSeq(seq)
	copy(msg.Buffer[HeadLen:HeadLen+len(method)], method)

	if len(data) == 0 {
		msg.body = nil
		msg.bodyPooled = false
	} else if owned {
		msg.body = data
		msg.bodyPooled = false
	} else {
		// The caller may modify the slice; keep a copy until it is written.
		b := h.Malloc(len(data))
		copy(b, data)
		msg.body = b
		msg.bodyPooled = true
	}

	return msg
}

// checkMethod returns an error if method is empty or longer than MaxMethodLen.
func checkMethod(method string) error {
	ml := len(method)
	if ml == 0 || ml > MaxMethodLen {
		return fmt.Errorf("invalid method length: %v, should <= %v", ml, MaxMethodLen)
	}
	return nil
}

// MessageCoder transforms Messages on the wire, e.g. for compression or
// encryption. Coders are applied in order on Encode and in reverse order on
// Decode.
type MessageCoder interface {
	// Encode transforms a Message before it is sent.
	Encode(*Client, *Message) *Message
	// Decode transforms a received Message before it is handled.
	Decode(*Client, *Message) *Message
}

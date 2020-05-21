package arpc

import (
	"encoding/binary"
	"errors"
	"fmt"
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
	m := Message(memPool.Get(HeadLen + l))
	copy(m, h)

	return m, nil
}

// Message defines rpc packet
type Message []byte

// Cmd returns message cmd
func (m Message) Cmd() byte {
	return m[headerIndexCmd]
}

// MethodLen returns message cmd
func (m Message) MethodLen() int {
	return int(m[headerIndexMethodLen])
}

// Seq returns message sequence
func (m Message) Seq() uint64 {
	return binary.LittleEndian.Uint64(m[headerIndexSeqBegin:headerIndexSeqEnd])
}

// Async returns async flag value
func (m Message) Async() byte {
	return m[headerIndexAsync]
}

// IsAsync returns async flag
func (m Message) IsAsync() bool {
	return m[headerIndexAsync] == 1
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

func newHeader() Header {
	return Header(make([]byte, HeadLen))
}

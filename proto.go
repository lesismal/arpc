package arpc

import (
	"encoding/binary"
	"errors"
)

const (
	// RPCCmdNone .
	RPCCmdNone byte = 0
	// RPCCmdReq .
	RPCCmdReq byte = 1
	// RPCCmdRsp .
	RPCCmdRsp byte = 2
	// RPCCmdErr .
	RPCCmdErr byte = 3
)

const (
	// HeadLen defines rpc packet's head length
	HeadLen int = 16

	// MaxBodyLen limit
	MaxBodyLen int = 1024 * 1024 * 64
)

// Header defines rpc head
type Header []byte

// BodyLen return length of message body
func (h Header) BodyLen() int {
	return int(binary.LittleEndian.Uint32(h[:4]))
}

// Message clones header with body length
func (h Header) Message() (Message, error) {
	l := h.BodyLen()
	if l == 0 {
		return Message(h), nil
	}
	if l < 0 || l > MaxBodyLen {
		return nil, ErrInvalidBodyLen
	}
	m := Message(make([]byte, HeadLen+l))
	copy(m, h)

	return m, nil
}

// Message defines rpc packet
type Message []byte

// Cmd return message cmd
func (m Message) Cmd() byte {
	return m[4]
}

// MethodLen return message cmd
func (m Message) MethodLen() int {
	return int(m[5])
}

// Seq return message seq/id
func (m Message) Seq() uint64 {
	return binary.LittleEndian.Uint64(m[8:16])
}

// Parse head and body, return cmd && seq && method && body && err
func (m Message) Parse() (byte, uint64, string, []byte, error) {
	cmd := m.Cmd()
	switch cmd {
	case RPCCmdReq:
		ml := m.MethodLen()
		// have methond, return method and body
		if ml > 0 {
			if len(m) >= HeadLen+ml {
				return cmd, m.Seq(), string(m[HeadLen : HeadLen+ml]), m[HeadLen+ml:], nil
			}
		} else {
			// no null method, return body
			return cmd, m.Seq(), "", m[HeadLen:], nil
		}
	case RPCCmdRsp:
		return cmd, m.Seq(), "", m[HeadLen:], nil
	case RPCCmdErr:
		return cmd, m.Seq(), "", nil, errors.New(string(m[HeadLen]))
	default:
	}

	return RPCCmdNone, m.Seq(), "", nil, ErrInvalidMessage
}

func newHeader() Header {
	return Header(make([]byte, HeadLen))
}

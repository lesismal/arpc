package coder

import (
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/util"
)

// Appender .
type Appender struct {
	AppenderName string
	FlagBitIndex int
	valueToBytes func(interface{}) ([]byte, error)
	bytesToValue func([]byte) (interface{}, error)
}

// Encode implements arpc MessageCoder.
func (ap *Appender) Encode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	if ap.AppenderName == "" || ap.valueToBytes == nil {
		return msg
	}
	key := ap.AppenderName
	value, ok := msg.Get(key)
	if !ok {
		return msg
	}
	if err := msg.SetFlagBit(ap.FlagBitIndex, true); err != nil {
		return msg
	}
	valueData, err := ap.valueToBytes(value)
	if err != nil {
		return msg
	}
	msg.Buffer = append(msg.Buffer, make([]byte, len(key)+len(valueData)+2)...)
	appendData := msg.Buffer[len(msg.Buffer)-len(key)-len(valueData)-2:]
	copy(appendData, key)
	copy(appendData[len(key):], valueData)
	appendLen := uint16(len(appendData))
	appendData[appendLen-2], appendData[appendLen-1] = byte(appendLen>>8), byte(appendLen&0xFF)
	msg.SetBodyLen(len(msg.Buffer) - 16)
	return msg
}

// Decode implements arpc MessageCoder.
func (ap *Appender) Decode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	if msg.IsFlagBitSet(ap.FlagBitIndex) {
		bufLen := len(msg.Buffer)
		if bufLen > 2 && ap.bytesToValue != nil {
			key := ap.AppenderName
			appendLen := (int(msg.Buffer[bufLen-2]) << 8) | int(msg.Buffer[bufLen-1])
			if bufLen >= appendLen {
				appenderName := util.BytesToStr(msg.Buffer[bufLen-appendLen : bufLen-appendLen+len(key)])
				if appenderName != key {
					return msg
				}
				payloadBody := msg.Buffer[bufLen-appendLen+len(appenderName) : bufLen-2]
				if value, err := ap.bytesToValue(payloadBody); err == nil {
					msg.Set(key, value)
				}
			}
			msg.Buffer = msg.Buffer[:len(msg.Buffer)-appendLen]
			msg.SetFlagBit(ap.FlagBitIndex, false)
			msg.SetBodyLen(len(msg.Buffer) - 16)
		}
	}
	return msg
}

// NewAppender returns the trace coding middleware.
func NewAppender(appenderName string,
	flagBitIndex int,
	valueToBytes func(interface{}) ([]byte, error),
	bytesToValue func([]byte) (interface{}, error)) *Appender {
	return &Appender{
		AppenderName: appenderName,
		FlagBitIndex: flagBitIndex,
		valueToBytes: valueToBytes,
		bytesToValue: bytesToValue,
	}
}

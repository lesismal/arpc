package msgpack

import (
	"github.com/lesismal/arpc"
	"github.com/vmihailenco/msgpack"
)

type mpkv struct {
	Body   []byte
	Values map[interface{}]interface{}
}

// MsgPack represents a gzip coding middleware.
type MsgPack int

// Encode implements arpc MessageCoder.
func (mp *MsgPack) Encode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	body := msg.Data()
	v := &mpkv{
		Body:   body,
		Values: msg.Values(),
	}
	data, err := msgpack.Marshal(&v)
	if err == nil {
		ml := msg.MethodLen()
		msg.Buffer = append(msg.Buffer[:arpc.HeadLen+ml], data...)
		msg.SetBodyLen(ml + len(data))
	} else {
		// some error log
	}
	return msg
}

// Decode implements arpc MessageCoder.
func (mp *MsgPack) Decode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	v := &mpkv{
		Body:   msg.Data(),
		Values: msg.Values(),
	}
	err := msgpack.Unmarshal(msg.Data(), v)
	if err == nil {
		ml := msg.MethodLen()
		msg.Buffer = append(msg.Buffer[:arpc.HeadLen+ml], v.Body...)
		msg.SetBodyLen(ml + len(v.Body))
		for k, v := range v.Values {
			msg.Set(k, v)
		}
	} else {
		// some error log
	}
	return msg
}

// New returns the MsgPack coding middleware.
func New() *MsgPack {
	return new(MsgPack)
}

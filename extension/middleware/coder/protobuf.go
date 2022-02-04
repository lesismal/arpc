//go:generate protoc --go_out=. ./protobuf/RPC.proto
package coder

import (
	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/coder/protobuf/RPC"
)
import "google.golang.org/protobuf/proto"

type ProtoBufCoder struct {
}

func (context *ProtoBufCoder) Encode(client *arpc.Client, message *arpc.Message) *arpc.Message {
	serialized, err := proto.Marshal(
		&RPC.Message{
			Cmd:      RPC.MessageType(message.Cmd()),
			Flag:     uint32(message.Buffer[arpc.HeaderIndexFlag]),
			Sequence: message.Seq(),
			Method:   message.Method(),
			Body:     message.Data(),
		},
	)
	if err != nil {
		return nil
	}

	bodyLen := len(serialized) - 12
	// fmt.Printf("%s: [% x]\n", "serialized", serialized)
	if len(message.Buffer) >= arpc.HeadLen+bodyLen {
		message.Buffer = message.Buffer[:arpc.HeadLen+bodyLen]
	} else {
		client.Handler.Free(message.Buffer)
		message.Buffer = client.Handler.Malloc(arpc.HeadLen + bodyLen)
	}

	message.SetBodyLen(bodyLen)
	copy(message.Buffer[arpc.HeaderIndexBodyLenEnd:], serialized)

	return message
}

func (context *ProtoBufCoder) Decode(client *arpc.Client, message *arpc.Message) *arpc.Message {
	decoded := RPC.Message{}
	err := proto.Unmarshal(message.Buffer[arpc.HeaderIndexBodyLenEnd:], &decoded)
	if err != nil {
		return nil
	}

	bodyLen := len(decoded.Method) + len(decoded.Body)
	if len(message.Buffer) >= arpc.HeadLen+bodyLen {
		message.Buffer = message.Buffer[:arpc.HeadLen+bodyLen]
	} else {
		client.Handler.Free(message.Buffer)
		message.Buffer = client.Handler.Malloc(arpc.HeadLen + bodyLen)
	}

	message.SetBodyLen(bodyLen)
	message.SetCmd(byte(decoded.Cmd))
	message.Buffer[arpc.HeaderIndexFlag] = byte(decoded.Flag)
	message.SetMethodLen(len(decoded.Method))
	message.SetSeq(decoded.Sequence)
	copy(message.Buffer[arpc.HeadLen:], decoded.Method)
	copy(message.Buffer[arpc.HeadLen+message.MethodLen():], decoded.Body)
	return message
}

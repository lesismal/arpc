package codec

import (
	protobuf "google.golang.org/protobuf/proto"
)

type ProtoBufCodec struct{}

func (context *ProtoBufCodec) Marshal(v interface{}) ([]byte, error) {
	return protobuf.Marshal(v.(protobuf.Message))
}

func (context *ProtoBufCodec) Unmarshal(data []byte, v interface{}) error {
	return protobuf.Unmarshal(data, v.(protobuf.Message))
}

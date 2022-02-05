package codec

import "google.golang.org/protobuf/proto"

type ProtoBufCodec struct{}

func (context *ProtoBufCodec) Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (context *ProtoBufCodec) Unmarshal(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}

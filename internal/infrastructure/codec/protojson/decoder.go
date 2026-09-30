package protojson

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type RequestDecoder struct{}

func NewRequestDecoder() RequestDecoder {
	return RequestDecoder{}
}

func (RequestDecoder) Decode(desc protoreflect.MessageDescriptor, data []byte) (proto.Message, error) {
	msg := dynamicpb.NewMessage(desc)
	if err := protojson.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("decode request json as %s: %w", desc.FullName(), err)
	}
	return msg, nil
}

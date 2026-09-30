package port

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type RequestDecoder interface {
	Decode(desc protoreflect.MessageDescriptor, data []byte) (proto.Message, error)
}

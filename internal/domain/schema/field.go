package schema

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type Field struct {
	descriptor protoreflect.FieldDescriptor
}

func (f Field) Descriptor() protoreflect.FieldDescriptor {
	return f.descriptor
}

func (f Field) Name() string {
	return string(f.descriptor.Name())
}

func (f Field) JSONName() string {
	return f.descriptor.JSONName()
}

func (f Field) Number() int {
	return int(f.descriptor.Number())
}

func (f Field) Kind() protoreflect.Kind {
	return f.descriptor.Kind()
}

func (f Field) IsList() bool {
	return f.descriptor.IsList()
}

func (f Field) IsMap() bool {
	return f.descriptor.IsMap()
}

func (f Field) IsOneof() bool {
	oneof := f.descriptor.ContainingOneof()
	return oneof != nil && !oneof.IsSynthetic()
}

func (f Field) OneofName() string {
	if !f.IsOneof() {
		return ""
	}
	return string(f.descriptor.ContainingOneof().Name())
}

func (f Field) HasPresence() bool {
	return f.descriptor.HasPresence()
}

func (f Field) IsOptional() bool {
	return f.descriptor.HasOptionalKeyword()
}

func (f Field) Message() (Message, bool) {
	if f.descriptor.IsMap() {
		return Message{}, false
	}
	md := f.descriptor.Message()
	if md == nil {
		return Message{}, false
	}
	return Message{descriptor: md}, true
}

func (f Field) Enum() (Enum, bool) {
	ed := f.descriptor.Enum()
	if ed == nil {
		return Enum{}, false
	}
	return Enum{descriptor: ed}, true
}

func (f Field) TypeName() string {
	if f.descriptor.IsMap() {
		return fmt.Sprintf("map<%s, %s>", kindName(f.descriptor.MapKey()), kindName(f.descriptor.MapValue()))
	}
	return kindName(f.descriptor)
}

func (f Field) Label() string {
	switch {
	case f.descriptor.IsMap():
		return ""
	case f.descriptor.IsList():
		return "repeated"
	case f.descriptor.HasOptionalKeyword():
		return "optional"
	default:
		return ""
	}
}

func kindName(fd protoreflect.FieldDescriptor) string {
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return string(fd.Message().FullName())
	case protoreflect.EnumKind:
		return string(fd.Enum().FullName())
	default:
		return fd.Kind().String()
	}
}

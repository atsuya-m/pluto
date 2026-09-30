package schema

import (
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type Message struct {
	descriptor protoreflect.MessageDescriptor
}

func NewMessage(descriptor protoreflect.MessageDescriptor) Message {
	return Message{descriptor: descriptor}
}

func (m Message) Descriptor() protoreflect.MessageDescriptor {
	return m.descriptor
}

func (m Message) Name() string {
	return string(m.descriptor.Name())
}

func (m Message) FullName() string {
	return string(m.descriptor.FullName())
}

func (m Message) Fields() []Field {
	fds := m.descriptor.Fields()
	fields := make([]Field, 0, fds.Len())
	for i := 0; i < fds.Len(); i++ {
		fields = append(fields, Field{descriptor: fds.Get(i)})
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Number() < fields[j].Number() })
	return fields
}

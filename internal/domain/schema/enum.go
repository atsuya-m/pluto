package schema

import "google.golang.org/protobuf/reflect/protoreflect"

type Enum struct {
	descriptor protoreflect.EnumDescriptor
}

type EnumValue struct {
	Name   string
	Number int32
}

func (e Enum) Descriptor() protoreflect.EnumDescriptor {
	return e.descriptor
}

func (e Enum) Name() string {
	return string(e.descriptor.Name())
}

func (e Enum) FullName() string {
	return string(e.descriptor.FullName())
}

func (e Enum) Values() []EnumValue {
	vds := e.descriptor.Values()
	values := make([]EnumValue, 0, vds.Len())
	for i := 0; i < vds.Len(); i++ {
		vd := vds.Get(i)
		values = append(values, EnumValue{Name: string(vd.Name()), Number: int32(vd.Number())})
	}
	return values
}

package schema

import "google.golang.org/protobuf/reflect/protoreflect"

type Service struct {
	descriptor protoreflect.ServiceDescriptor
}

func (s Service) Name() string {
	return string(s.descriptor.Name())
}

func (s Service) FullName() string {
	return string(s.descriptor.FullName())
}

func (s Service) RPCs() []RPC {
	methods := s.descriptor.Methods()
	rpcs := make([]RPC, 0, methods.Len())
	for i := 0; i < methods.Len(); i++ {
		rpcs = append(rpcs, RPC{descriptor: methods.Get(i)})
	}
	return rpcs
}

package schema

import "google.golang.org/protobuf/reflect/protoreflect"

type RPC struct {
	descriptor protoreflect.MethodDescriptor
}

func (r RPC) Descriptor() protoreflect.MethodDescriptor {
	return r.descriptor
}

func (r RPC) Name() string {
	return string(r.descriptor.Name())
}

func (r RPC) FullName() string {
	return string(r.descriptor.FullName())
}

func (r RPC) ServiceName() string {
	return string(r.descriptor.Parent().FullName())
}

func (r RPC) ShortServiceName() string {
	return string(r.descriptor.Parent().Name())
}

func (r RPC) Procedure() string {
	return "/" + r.ServiceName() + "/" + r.Name()
}

func (r RPC) Input() Message {
	return Message{descriptor: r.descriptor.Input()}
}

func (r RPC) Output() Message {
	return Message{descriptor: r.descriptor.Output()}
}

func (r RPC) ClientStreaming() bool {
	return r.descriptor.IsStreamingClient()
}

func (r RPC) ServerStreaming() bool {
	return r.descriptor.IsStreamingServer()
}

func (r RPC) Unary() bool {
	return !r.ClientStreaming() && !r.ServerStreaming()
}

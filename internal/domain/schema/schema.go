package schema

import (
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type Schema struct {
	files *protoregistry.Files

	services []Service
	rpcs     []RPC
	messages []Message
	enums    []Enum
}

type Option func(*options)

type options struct {
	services map[string]bool
}

func OnlyServices(names ...string) Option {
	return func(o *options) {
		o.services = map[string]bool{}
		for _, n := range names {
			o.services[n] = true
		}
	}
}

func New(files *protoregistry.Files, opts ...Option) (*Schema, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	s := &Schema{files: files}

	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Services().Len(); i++ {
			sd := fd.Services().Get(i)
			if o.services != nil && !o.services[string(sd.FullName())] {
				continue
			}
			s.services = append(s.services, Service{descriptor: sd})
			for j := 0; j < sd.Methods().Len(); j++ {
				s.rpcs = append(s.rpcs, RPC{descriptor: sd.Methods().Get(j)})
			}
		}
		s.collectMessages(fd.Messages())
		s.collectEnums(fd.Enums())
		return true
	})

	sort.Slice(s.services, func(i, j int) bool { return s.services[i].FullName() < s.services[j].FullName() })
	sort.Slice(s.rpcs, func(i, j int) bool { return s.rpcs[i].FullName() < s.rpcs[j].FullName() })
	sort.Slice(s.messages, func(i, j int) bool { return s.messages[i].FullName() < s.messages[j].FullName() })
	sort.Slice(s.enums, func(i, j int) bool { return s.enums[i].FullName() < s.enums[j].FullName() })

	return s, nil
}

func (s *Schema) collectMessages(mds protoreflect.MessageDescriptors) {
	for i := 0; i < mds.Len(); i++ {
		md := mds.Get(i)
		if md.IsMapEntry() {
			continue
		}
		s.messages = append(s.messages, Message{descriptor: md})
		s.collectMessages(md.Messages())
		s.collectEnums(md.Enums())
	}
}

func (s *Schema) collectEnums(eds protoreflect.EnumDescriptors) {
	for i := 0; i < eds.Len(); i++ {
		s.enums = append(s.enums, Enum{descriptor: eds.Get(i)})
	}
}

func (s *Schema) Files() *protoregistry.Files {
	return s.files
}

func (s *Schema) Services() []Service {
	return append([]Service(nil), s.services...)
}

func (s *Schema) RPCs() []RPC {
	return append([]RPC(nil), s.rpcs...)
}

func (s *Schema) ResolveService(name string) (Service, error) {
	return resolve("service", name, s.services)
}

func (s *Schema) ResolveRPC(name string) (RPC, error) {
	return resolve("rpc", name, s.rpcs)
}

func (s *Schema) ResolveMessage(name string) (Message, error) {
	return resolve("message", name, s.messages)
}

func (s *Schema) ResolveEnum(name string) (Enum, error) {
	return resolve("enum", name, s.enums)
}

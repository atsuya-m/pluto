package proto

import (
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func registerWithDeps(registry *protoregistry.Files, fd protoreflect.FileDescriptor) error {
	if _, err := registry.FindFileByPath(fd.Path()); err == nil {
		return nil
	}
	imports := fd.Imports()
	for i := 0; i < imports.Len(); i++ {
		if err := registerWithDeps(registry, imports.Get(i).FileDescriptor); err != nil {
			return err
		}
	}
	return registry.RegisterFile(fd)
}

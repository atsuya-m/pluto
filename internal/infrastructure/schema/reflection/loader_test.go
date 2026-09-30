package reflection_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/atsuya-m/pluto/internal/infrastructure/schema/reflection"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
	"github.com/atsuya-m/pluto/internal/testutil/testserver"
)

type fakeClient struct {
	services  []string
	files     map[string]*descriptorpb.FileDescriptorProto
	bySymbol  map[string][]string
	listErr   error
	byNameErr bool
}

func (f *fakeClient) ListServices(context.Context) ([]string, error) {
	return f.services, f.listErr
}

func (f *fakeClient) FileContainingSymbol(_ context.Context, symbol string) ([]*descriptorpb.FileDescriptorProto, error) {
	names, ok := f.bySymbol[symbol]
	if !ok {
		return nil, errors.New("symbol not found")
	}
	var out []*descriptorpb.FileDescriptorProto
	for _, n := range names {
		out = append(out, f.files[n])
	}
	return out, nil
}

func (f *fakeClient) FileByFilename(_ context.Context, name string) ([]*descriptorpb.FileDescriptorProto, error) {
	if f.byNameErr {
		return nil, errors.New("not found")
	}
	fdp, ok := f.files[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return []*descriptorpb.FileDescriptorProto{fdp}, nil
}

func fixtureFiles(t *testing.T) map[string]*descriptorpb.FileDescriptorProto {
	files := map[string]*descriptorpb.FileDescriptorProto{}
	fixture.Registry(t).RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		files[fd.Path()] = protodesc.ToFileDescriptorProto(fd)
		return true
	})
	return files
}

func rpcNames(t *testing.T, client reflection.Client) []string {
	t.Helper()
	s, err := reflection.NewSchemaLoader(client).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var names []string
	for _, r := range s.RPCs() {
		names = append(names, r.FullName())
	}
	return names
}

func TestSchemaLoader_ResolvesServicesAndSkipsReflection(t *testing.T) {
	client := &fakeClient{
		services: []string{"grpc.reflection.v1.ServerReflection", "user.v1.UserService"},
		files:    fixtureFiles(t),
		bySymbol: map[string][]string{"user.v1.UserService": {"user/v1/user.proto", "google/protobuf/timestamp.proto"}},
	}
	names := rpcNames(t, client)
	if len(names) != 8 || !slices.Contains(names, "user.v1.UserService.CreateUser") {
		t.Errorf("rpcs = %v", names)
	}
}

func TestSchemaLoader_FetchesMissingDependencies(t *testing.T) {
	client := &fakeClient{
		services: []string{"admin.v1.AdminService"},
		files:    fixtureFiles(t),
		bySymbol: map[string][]string{"admin.v1.AdminService": {"admin/v1/admin.proto"}},
	}
	if names := rpcNames(t, client); len(names) != 2 {
		t.Errorf("rpcs = %v", names)
	}
}

func TestSchemaLoader_FallsBackToWellKnownTypes(t *testing.T) {
	client := &fakeClient{
		services:  []string{"user.v1.UserService"},
		files:     fixtureFiles(t),
		bySymbol:  map[string][]string{"user.v1.UserService": {"user/v1/user.proto"}},
		byNameErr: true,
	}
	if names := rpcNames(t, client); len(names) != 8 {
		t.Errorf("rpcs = %v", names)
	}
}

func TestSchemaLoader_Errors(t *testing.T) {
	ctx := context.Background()

	_, err := reflection.NewSchemaLoader(&fakeClient{listErr: errors.New("unimplemented")}).Load(ctx)
	if err == nil || !strings.Contains(err.Error(), "list services") {
		t.Errorf("list error = %v", err)
	}

	_, err = reflection.NewSchemaLoader(&fakeClient{services: []string{"x.Missing"}}).Load(ctx)
	if err == nil || !strings.Contains(err.Error(), "resolve x.Missing") {
		t.Errorf("resolve error = %v", err)
	}

	files := fixtureFiles(t)
	_, err = reflection.NewSchemaLoader(&fakeClient{
		services:  []string{"admin.v1.AdminService"},
		files:     map[string]*descriptorpb.FileDescriptorProto{"admin/v1/admin.proto": files["admin/v1/admin.proto"]},
		bySymbol:  map[string][]string{"admin.v1.AdminService": {"admin/v1/admin.proto"}},
		byNameErr: true,
	}).Load(ctx)
	if err == nil || !strings.Contains(err.Error(), "user/v1/user.proto") {
		t.Errorf("missing dependency error = %v", err)
	}
}

func TestGRPCClient_AgainstServer(t *testing.T) {
	srv := testserver.New(t)
	client, err := reflection.NewGRPCClient(srv.URL, http.Header{"Authorization": {"Bearer x"}})
	if err != nil {
		t.Fatal(err)
	}
	names := rpcNames(t, client)
	if len(names) != 10 {
		t.Errorf("rpcs = %v", names)
	}
}

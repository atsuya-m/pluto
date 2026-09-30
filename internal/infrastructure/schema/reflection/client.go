package reflection

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/atsuya-m/pluto/internal/infrastructure/transport/httpclient"
)

type GRPCClient struct {
	client  *grpcreflect.Client
	headers http.Header
}

func NewGRPCClient(target string, headers http.Header) (*GRPCClient, error) {
	u, err := httpclient.ParseTarget(target)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(u.String(), "/")
	return &GRPCClient{
		client:  grpcreflect.NewClient(httpclient.NewHTTP2(u), base, connect.WithGRPC()),
		headers: headers,
	}, nil
}

func (c *GRPCClient) stream(ctx context.Context) *grpcreflect.ClientStream {
	return c.client.NewStream(ctx, grpcreflect.WithRequestHeaders(c.headers))
}

func (c *GRPCClient) ListServices(ctx context.Context) ([]string, error) {
	s := c.stream(ctx)
	defer closeStream(s)
	names, err := s.ListServices()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, string(n))
	}
	return out, nil
}

func (c *GRPCClient) FileContainingSymbol(ctx context.Context, symbol string) ([]*descriptorpb.FileDescriptorProto, error) {
	s := c.stream(ctx)
	defer closeStream(s)
	return s.FileContainingSymbol(protoreflect.FullName(symbol))
}

func (c *GRPCClient) FileByFilename(ctx context.Context, filename string) ([]*descriptorpb.FileDescriptorProto, error) {
	s := c.stream(ctx)
	defer closeStream(s)
	return s.FileByFilename(filename)
}

func closeStream(s *grpcreflect.ClientStream) {
	_, _ = s.Close()
}

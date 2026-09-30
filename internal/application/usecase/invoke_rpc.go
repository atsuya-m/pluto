package usecase

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/application/port"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

func streamKind(r schema.RPC) string {
	switch {
	case r.ClientStreaming() && r.ServerStreaming():
		return "bidi streaming"
	case r.ClientStreaming():
		return "client streaming"
	case r.ServerStreaming():
		return "server streaming"
	default:
		return "unary"
	}
}

type InvokeRPC struct {
	schemaLoader port.SchemaLoader
	invoker      port.RPCInvoker
}

type InvokeRPCInput struct {
	RPCName string
	Message proto.Message
	Headers http.Header
}

type InvokeRPCOutput struct {
	RPC      RPCSummary
	Message  proto.Message
	Headers  http.Header
	Trailers http.Header
}

func NewInvokeRPC(schemaLoader port.SchemaLoader, invoker port.RPCInvoker) *InvokeRPC {
	return &InvokeRPC{schemaLoader: schemaLoader, invoker: invoker}
}

func (u *InvokeRPC) Execute(ctx context.Context, in InvokeRPCInput) (InvokeRPCOutput, error) {
	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return InvokeRPCOutput{}, err
	}
	r, err := s.ResolveRPC(in.RPCName)
	if err != nil {
		return InvokeRPCOutput{}, err
	}
	if !r.Unary() {
		return InvokeRPCOutput{}, fmt.Errorf("rpc %s is %s, not unary", r.FullName(), streamKind(r))
	}
	if in.Message == nil {
		return InvokeRPCOutput{}, fmt.Errorf("request message is required")
	}
	if got, want := in.Message.ProtoReflect().Descriptor().FullName(), r.Input().Descriptor().FullName(); got != want {
		return InvokeRPCOutput{}, fmt.Errorf("request message type mismatch: got %s, want %s", got, want)
	}

	res, err := u.invoker.Invoke(ctx, invocation.Request{
		RPC:     r,
		Message: in.Message,
		Headers: in.Headers,
	})
	if err != nil {
		return InvokeRPCOutput{}, err
	}
	return InvokeRPCOutput{
		RPC:      toRPCSummary(r),
		Message:  res.Message,
		Headers:  res.Headers,
		Trailers: res.Trailers,
	}, nil
}

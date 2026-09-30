package usecase

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/application/port"
)

type InvokeServerStream struct {
	schemaLoader port.SchemaLoader
	invoker      port.ServerStreamInvoker
}

type InvokeServerStreamInput struct {
	RPCName   string
	Message   proto.Message
	Headers   http.Header
	OnMessage func(proto.Message) error
}

type InvokeServerStreamOutput struct {
	RPC      RPCSummary
	Count    int
	Headers  http.Header
	Trailers http.Header
}

func NewInvokeServerStream(schemaLoader port.SchemaLoader, invoker port.ServerStreamInvoker) *InvokeServerStream {
	return &InvokeServerStream{schemaLoader: schemaLoader, invoker: invoker}
}

func (u *InvokeServerStream) Execute(ctx context.Context, in InvokeServerStreamInput) (InvokeServerStreamOutput, error) {
	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return InvokeServerStreamOutput{}, err
	}
	r, err := s.ResolveRPC(in.RPCName)
	if err != nil {
		return InvokeServerStreamOutput{}, err
	}
	if !r.ServerStreaming() || r.ClientStreaming() {
		return InvokeServerStreamOutput{}, fmt.Errorf("rpc %s is %s, not server streaming", r.FullName(), streamKind(r))
	}
	if in.Message == nil {
		return InvokeServerStreamOutput{}, fmt.Errorf("request message is required")
	}
	if got, want := in.Message.ProtoReflect().Descriptor().FullName(), r.Input().Descriptor().FullName(); got != want {
		return InvokeServerStreamOutput{}, fmt.Errorf("request message type mismatch: got %s, want %s", got, want)
	}

	out := InvokeServerStreamOutput{RPC: toRPCSummary(r)}
	res, err := u.invoker.InvokeServerStream(ctx, invocation.Request{
		RPC:     r,
		Message: in.Message,
		Headers: in.Headers,
	}, func(msg proto.Message) error {
		out.Count++
		if in.OnMessage == nil {
			return nil
		}
		return in.OnMessage(msg)
	})
	out.Headers, out.Trailers = res.Headers, res.Trailers
	return out, err
}

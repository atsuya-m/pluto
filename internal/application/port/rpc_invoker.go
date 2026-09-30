package port

import (
	"context"

	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/application/invocation"
)

type RPCInvoker interface {
	Invoke(ctx context.Context, req invocation.Request) (invocation.Response, error)
}

type ServerStreamInvoker interface {
	InvokeServerStream(ctx context.Context, req invocation.Request, onMessage func(proto.Message) error) (invocation.StreamResult, error)
}

type StreamOpener interface {
	OpenStream(ctx context.Context, req invocation.Request) (invocation.Stream, error)
}

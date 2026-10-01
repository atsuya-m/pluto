package bootstrap

import (
	"net/http"

	"github.com/atsuya-m/pluto/internal/application/port"
	"github.com/atsuya-m/pluto/internal/application/usecase"
	"github.com/atsuya-m/pluto/internal/infrastructure/codec/protojson"
	"github.com/atsuya-m/pluto/internal/infrastructure/schema/cached"
	"github.com/atsuya-m/pluto/internal/infrastructure/schema/proto"
	"github.com/atsuya-m/pluto/internal/infrastructure/schema/reflection"
	"github.com/atsuya-m/pluto/internal/infrastructure/transport/connect"
)

type Application struct {
	Config  Config
	Headers http.Header

	ListServices    *usecase.ListServices
	ListRPCs        *usecase.ListRPCs
	DescribeRPC     *usecase.DescribeRPC
	DescribeMessage *usecase.DescribeMessage
	PrepareRequest  *usecase.PrepareRequest
	InvokeRPC       *usecase.InvokeRPC
	InvokeStream    *usecase.InvokeServerStream
	OpenStream      *usecase.OpenStream
}

func New(cfg Config) (*Application, error) {
	headers, err := cfg.HTTPHeaders()
	if err != nil {
		return nil, err
	}
	protocol, err := connect.ParseProtocol(cfg.Protocol)
	if err != nil {
		return nil, err
	}

	var source port.SchemaLoader = proto.NewSchemaLoader(cfg.SchemaPaths, cfg.ImportPaths)
	if cfg.Reflection {
		client, err := reflection.NewGRPCClient(cfg.Target, headers)
		if err != nil {
			return nil, err
		}
		source = reflection.NewSchemaLoader(client)
	}
	loader := cached.NewSchemaLoader(source)
	invoker, err := connect.NewInvoker(cfg.Target, connect.WithProtocol(protocol), connect.WithJSON(cfg.JSONCodec))
	if err != nil {
		return nil, err
	}
	decoder := protojson.NewRequestDecoder()

	return &Application{
		Config:          cfg,
		Headers:         headers,
		ListServices:    usecase.NewListServices(loader),
		ListRPCs:        usecase.NewListRPCs(loader),
		DescribeRPC:     usecase.NewDescribeRPC(loader),
		DescribeMessage: usecase.NewDescribeMessage(loader),
		PrepareRequest:  usecase.NewPrepareRequest(loader, decoder),
		InvokeRPC:       usecase.NewInvokeRPC(loader, invoker),
		InvokeStream:    usecase.NewInvokeServerStream(loader, invoker),
		OpenStream:      usecase.NewOpenStream(loader, invoker),
	}, nil
}

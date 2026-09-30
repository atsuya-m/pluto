package usecase

import (
	"context"

	"github.com/atsuya-m/pluto/internal/application/port"
)

type DescribeRPC struct {
	schemaLoader port.SchemaLoader
}

type DescribeRPCInput struct {
	Name string
}

type DescribeRPCOutput struct {
	RPC RPCDetail
}

func NewDescribeRPC(schemaLoader port.SchemaLoader) *DescribeRPC {
	return &DescribeRPC{schemaLoader: schemaLoader}
}

func (u *DescribeRPC) Execute(ctx context.Context, in DescribeRPCInput) (DescribeRPCOutput, error) {
	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return DescribeRPCOutput{}, err
	}
	r, err := s.ResolveRPC(in.Name)
	if err != nil {
		return DescribeRPCOutput{}, err
	}
	return DescribeRPCOutput{RPC: RPCDetail{
		RPCSummary: toRPCSummary(r),
		Request:    toMessageDetail(r.Input()),
		Response:   toMessageDetail(r.Output()),
	}}, nil
}

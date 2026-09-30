package usecase

import (
	"context"

	"github.com/atsuya-m/pluto/internal/application/port"
)

type ListRPCs struct {
	schemaLoader port.SchemaLoader
}

type ListRPCsInput struct {
	Service string
}

type ListRPCsOutput struct {
	RPCs []RPCSummary
}

func NewListRPCs(schemaLoader port.SchemaLoader) *ListRPCs {
	return &ListRPCs{schemaLoader: schemaLoader}
}

func (u *ListRPCs) Execute(ctx context.Context, in ListRPCsInput) (ListRPCsOutput, error) {
	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return ListRPCsOutput{}, err
	}

	rpcs := s.RPCs()
	if in.Service != "" {
		svc, err := s.ResolveService(in.Service)
		if err != nil {
			return ListRPCsOutput{}, err
		}
		rpcs = svc.RPCs()
	}

	out := ListRPCsOutput{RPCs: make([]RPCSummary, 0, len(rpcs))}
	for _, r := range rpcs {
		out.RPCs = append(out.RPCs, toRPCSummary(r))
	}
	return out, nil
}

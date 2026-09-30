package usecase

import (
	"context"

	"github.com/atsuya-m/pluto/internal/application/port"
)

type ListServices struct {
	schemaLoader port.SchemaLoader
}

type ListServicesInput struct{}

type ListServicesOutput struct {
	Services []ServiceSummary
}

func NewListServices(schemaLoader port.SchemaLoader) *ListServices {
	return &ListServices{schemaLoader: schemaLoader}
}

func (u *ListServices) Execute(ctx context.Context, _ ListServicesInput) (ListServicesOutput, error) {
	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return ListServicesOutput{}, err
	}
	services := s.Services()
	out := ListServicesOutput{Services: make([]ServiceSummary, 0, len(services))}
	for _, svc := range services {
		out.Services = append(out.Services, toServiceSummary(svc))
	}
	return out, nil
}

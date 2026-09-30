package usecase

import (
	"context"

	"github.com/atsuya-m/pluto/internal/application/port"
)

type DescribeMessage struct {
	schemaLoader port.SchemaLoader
}

type DescribeMessageInput struct {
	Name string
}

type DescribeMessageOutput struct {
	Message MessageDetail
}

func NewDescribeMessage(schemaLoader port.SchemaLoader) *DescribeMessage {
	return &DescribeMessage{schemaLoader: schemaLoader}
}

func (u *DescribeMessage) Execute(ctx context.Context, in DescribeMessageInput) (DescribeMessageOutput, error) {
	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return DescribeMessageOutput{}, err
	}
	m, err := s.ResolveMessage(in.Name)
	if err != nil {
		return DescribeMessageOutput{}, err
	}
	return DescribeMessageOutput{Message: toMessageDetail(m)}, nil
}

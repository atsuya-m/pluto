package app

import (
	"context"
	"net/http"

	"github.com/atsuya-m/pluto/internal/application/usecase"
)

type ListServicesUseCase interface {
	Execute(context.Context, usecase.ListServicesInput) (usecase.ListServicesOutput, error)
}

type ListRPCsUseCase interface {
	Execute(context.Context, usecase.ListRPCsInput) (usecase.ListRPCsOutput, error)
}

type DescribeRPCUseCase interface {
	Execute(context.Context, usecase.DescribeRPCInput) (usecase.DescribeRPCOutput, error)
}

type DescribeMessageUseCase interface {
	Execute(context.Context, usecase.DescribeMessageInput) (usecase.DescribeMessageOutput, error)
}

type PrepareRequestUseCase interface {
	Execute(context.Context, usecase.PrepareRequestInput) (usecase.PrepareRequestOutput, error)
}

type InvokeRPCUseCase interface {
	Execute(context.Context, usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error)
}

type InvokeServerStreamUseCase interface {
	Execute(context.Context, usecase.InvokeServerStreamInput) (usecase.InvokeServerStreamOutput, error)
}

type OpenStreamUseCase interface {
	Execute(context.Context, usecase.OpenStreamInput) (usecase.OpenStreamOutput, error)
}

type SaveRequestUseCase interface {
	Execute(context.Context, usecase.SaveRequestInput) error
}

type ListSavedRequestsUseCase interface {
	Execute(context.Context) ([]usecase.SavedRequestSummary, error)
}

type DeleteSavedRequestUseCase interface {
	Execute(context.Context, string) error
}

type Dependencies struct {
	ListServices    ListServicesUseCase
	ListRPCs        ListRPCsUseCase
	DescribeRPC     DescribeRPCUseCase
	DescribeMessage DescribeMessageUseCase
	PrepareRequest  PrepareRequestUseCase
	InvokeRPC       InvokeRPCUseCase
	InvokeStream    InvokeServerStreamUseCase
	OpenStream      OpenStreamUseCase
	SaveRequest     SaveRequestUseCase
	ListSaved       ListSavedRequestsUseCase
	DeleteSaved     DeleteSavedRequestUseCase

	Headers      http.Header
	Target       string
	Protocol     string
	SchemaSource string
	Profile      string

	Clipboard func(text string) (method string, err error)
}

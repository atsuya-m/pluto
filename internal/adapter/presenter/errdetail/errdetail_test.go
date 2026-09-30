package errdetail_test

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/atsuya-m/pluto/internal/adapter/presenter/errdetail"
	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

func TestFrom(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		kind     errdetail.Kind
		code     string
		message  string
		extraLen int
	}{
		{"not found", &schema.NotFoundError{Kind: "rpc", Query: "X"}, errdetail.KindNotFound, "not_found", `rpc "X" not found`, 0},
		{"wrapped not found", fmt.Errorf("wrap: %w", &schema.NotFoundError{Kind: "rpc", Query: "X"}), errdetail.KindNotFound, "not_found", `wrap: rpc "X" not found`, 0},
		{"ambiguous", &schema.AmbiguousSymbolError{Query: "Get", Candidates: []string{"a.Get", "b.Get"}}, errdetail.KindAmbiguous, "ambiguous_symbol", `ambiguous symbol "Get" matches 2 candidates`, 2},
		{"rpc", &invocation.Error{Code: "not_found", Message: "user missing", Details: []string{"d1"}}, errdetail.KindRPC, "not_found", "user missing", 1},
		{"other", errors.New("boom"), errdetail.KindInternal, "error", "boom", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := errdetail.From(tt.err)
			if d.Kind != tt.kind || d.Code != tt.code || d.Message != tt.message {
				t.Errorf("From = %+v", d)
			}
			if got := len(d.Candidates) + len(d.Details); got != tt.extraLen {
				t.Errorf("candidates+details = %d, want %d", got, tt.extraLen)
			}
		})
	}

	d := errdetail.From(&schema.AmbiguousSymbolError{Query: "Get", Candidates: []string{"a.Get", "b.Get"}})
	if !slices.Equal(d.Candidates, []string{"a.Get", "b.Get"}) {
		t.Errorf("candidates = %v", d.Candidates)
	}
}

func TestFrom_FiltersTransportMetadata(t *testing.T) {
	d := errdetail.From(&invocation.Error{
		Code:     "not_found",
		Headers:  http.Header{"Date": {"x"}, "Content-Type": {"application/grpc"}, "X-Reason": {"1"}},
		Trailers: http.Header{"Grpc-Status": {"5"}, "Connect-Timeout-Ms": {"1"}, "X-Param": {"id"}},
	})
	if len(d.Metadata) != 2 || d.Metadata["X-Reason"][0] != "1" || d.Metadata["X-Param"][0] != "id" {
		t.Errorf("metadata = %v", d.Metadata)
	}
	if errdetail.From(&invocation.Error{Headers: http.Header{"Date": {"x"}}}).Metadata != nil {
		t.Error("metadata with only transport headers should be omitted")
	}
}

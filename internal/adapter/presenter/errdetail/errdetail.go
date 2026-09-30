package errdetail

import (
	"errors"
	"net/http"
	"strings"

	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type Kind int

const (
	KindInternal Kind = iota
	KindNotFound
	KindAmbiguous
	KindInvalidArgument
	KindRPC
)

type ErrorDetail struct {
	Kind       Kind                `json:"-"`
	Code       string              `json:"code"`
	Message    string              `json:"message"`
	Candidates []string            `json:"candidates,omitempty"`
	Details    []string            `json:"details,omitempty"`
	Metadata   map[string][]string `json:"metadata,omitempty"`
}

func From(err error) ErrorDetail {
	var ambiguous *schema.AmbiguousSymbolError
	var invErr *invocation.Error

	switch {
	case errors.As(err, &ambiguous):
		return ErrorDetail{Kind: KindAmbiguous, Code: "ambiguous_symbol", Message: err.Error(), Candidates: ambiguous.Candidates}
	case errors.Is(err, schema.ErrSymbolNotFound):
		return ErrorDetail{Kind: KindNotFound, Code: "not_found", Message: err.Error()}
	case errors.As(err, &invErr):
		return ErrorDetail{Kind: KindRPC, Code: invErr.Code, Message: invErr.Message, Details: invErr.Details, Metadata: applicationMetadata(invErr.Headers, invErr.Trailers)}
	default:
		return ErrorDetail{Kind: KindInternal, Code: "error", Message: err.Error()}
	}
}

var transportHeaders = map[string]bool{
	"Date": true, "Server": true, "Via": true, "Vary": true, "Alt-Svc": true,
	"Content-Type": true, "Content-Length": true, "Content-Encoding": true,
	"Accept-Encoding": true, "Trailer": true, "Te": true,
}

func applicationMetadata(sources ...http.Header) map[string][]string {
	out := map[string][]string{}
	for _, h := range sources {
		for k, vs := range h {
			k = http.CanonicalHeaderKey(k)
			if transportHeaders[k] || strings.HasPrefix(k, "Grpc-") || strings.HasPrefix(k, "Connect-") {
				continue
			}
			out[k] = append(out[k], vs...)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

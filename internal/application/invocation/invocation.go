package invocation

import (
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type Request struct {
	RPC     schema.RPC
	Message proto.Message
	Headers http.Header
}

type Response struct {
	Message  proto.Message
	Headers  http.Header
	Trailers http.Header
}

type StreamResult struct {
	Headers  http.Header
	Trailers http.Header
}

type Error struct {
	Code     string
	Message  string
	Details  []string
	Headers  http.Header
	Trailers http.Header
	Cause    error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

type Stream interface {
	Send(msg proto.Message) error
	CloseSend() error
	Receive() (proto.Message, error)
	ResponseHeaders() http.Header
	ResponseTrailers() http.Header
	Close() error
}

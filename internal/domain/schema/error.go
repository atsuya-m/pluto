package schema

import (
	"errors"
	"fmt"
)

var ErrSymbolNotFound = errors.New("symbol not found")

type AmbiguousSymbolError struct {
	Query      string
	Candidates []string
}

func (e *AmbiguousSymbolError) Error() string {
	return fmt.Sprintf("ambiguous symbol %q matches %d candidates", e.Query, len(e.Candidates))
}

type NotFoundError struct {
	Kind  string
	Query string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Kind, e.Query)
}

func (e *NotFoundError) Unwrap() error {
	return ErrSymbolNotFound
}

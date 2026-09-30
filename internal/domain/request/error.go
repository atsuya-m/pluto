package request

import (
	"errors"
	"fmt"
)

var (
	ErrEmptyPath       = errors.New("field path is empty")
	ErrNotSingularMsg  = errors.New("intermediate field is not a singular message")
	ErrUnsupportedKind = errors.New("unsupported field kind for scalar input")
)

type UnknownFieldError struct {
	Message string
	Field   string
}

func (e *UnknownFieldError) Error() string {
	return fmt.Sprintf("message %s has no field %q", e.Message, e.Field)
}

type InvalidValueError struct {
	Field string
	Input string
	Err   error
}

func (e *InvalidValueError) Error() string {
	return fmt.Sprintf("invalid value %q for field %s: %v", e.Input, e.Field, e.Err)
}

func (e *InvalidValueError) Unwrap() error {
	return e.Err
}

type IndexOutOfRangeError struct {
	Field string
	Index int
	Len   int
}

func (e *IndexOutOfRangeError) Error() string {
	return fmt.Sprintf("index %d out of range for %s (len %d)", e.Index, e.Field, e.Len)
}

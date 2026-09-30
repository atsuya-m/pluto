package request

import (
	"strconv"
	"strings"
)

type stepKind int

const (
	stepField stepKind = iota
	stepIndex
	stepKey
)

type Step struct {
	Field string
	Index int
	Key   string
	kind  stepKind
}

func (s Step) IsIndex() bool {
	return s.kind == stepIndex
}

func (s Step) IsKey() bool {
	return s.kind == stepKey
}

func (s Step) String() string {
	switch s.kind {
	case stepIndex:
		return s.Field + "[" + strconv.Itoa(s.Index) + "]"
	case stepKey:
		return s.Field + "[" + strconv.Quote(s.Key) + "]"
	default:
		return s.Field
	}
}

type FieldPath []Step

func Path(fields ...string) FieldPath {
	p := make(FieldPath, 0, len(fields))
	for _, f := range fields {
		p = append(p, Step{Field: f})
	}
	return p
}

func (p FieldPath) clone(extra int) FieldPath {
	next := make(FieldPath, len(p), len(p)+extra)
	copy(next, p)
	return next
}

func (p FieldPath) Append(field string) FieldPath {
	return append(p.clone(1), Step{Field: field})
}

func (p FieldPath) AtIndex(i int) FieldPath {
	next := p.clone(0)
	if len(next) > 0 {
		next[len(next)-1] = Step{Field: next[len(next)-1].Field, Index: i, kind: stepIndex}
	}
	return next
}

func (p FieldPath) AtKey(key string) FieldPath {
	next := p.clone(0)
	if len(next) > 0 {
		next[len(next)-1] = Step{Field: next[len(next)-1].Field, Key: key, kind: stepKey}
	}
	return next
}

func (p FieldPath) Container() FieldPath {
	next := p.clone(0)
	if len(next) > 0 {
		next[len(next)-1] = Step{Field: next[len(next)-1].Field}
	}
	return next
}

func (p FieldPath) Parent() FieldPath {
	if len(p) == 0 {
		return nil
	}
	return p[:len(p)-1]
}

func (p FieldPath) Last() Step {
	if len(p) == 0 {
		return Step{}
	}
	return p[len(p)-1]
}

func (p FieldPath) String() string {
	parts := make([]string, 0, len(p))
	for _, s := range p {
		parts = append(parts, s.String())
	}
	return strings.Join(parts, ".")
}

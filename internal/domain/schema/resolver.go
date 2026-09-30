package schema

import (
	"sort"
	"strings"
)

type symbol interface {
	FullName() string
}

func normalizeQuery(query string) string {
	q := strings.TrimSpace(query)
	q = strings.TrimPrefix(q, "/")
	q = strings.ReplaceAll(q, "/", ".")
	return strings.TrimPrefix(q, ".")
}

func resolve[T symbol](kind, query string, items []T) (T, error) {
	var zero T
	q := normalizeQuery(query)
	if q == "" {
		return zero, &NotFoundError{Kind: kind, Query: query}
	}

	for _, item := range items {
		if item.FullName() == q {
			return item, nil
		}
	}

	var candidates []T
	suffix := "." + q
	for _, item := range items {
		if strings.HasSuffix(item.FullName(), suffix) {
			candidates = append(candidates, item)
		}
	}

	switch len(candidates) {
	case 0:
		return zero, &NotFoundError{Kind: kind, Query: query}
	case 1:
		return candidates[0], nil
	default:
		names := make([]string, 0, len(candidates))
		for _, c := range candidates {
			names = append(names, c.FullName())
		}
		sort.Strings(names)
		return zero, &AmbiguousSymbolError{Query: query, Candidates: names}
	}
}

package app

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

var errHeaderUsage = errors.New("usage: header [set|add] <Key> <value...> | header rm <Key> | header clear")

func applyHeaderCommand(current http.Header, args []string) (http.Header, string, error) {
	h := current.Clone()
	if h == nil {
		h = http.Header{}
	}
	if len(args) == 0 {
		return h, "", errHeaderUsage
	}
	switch strings.ToLower(args[0]) {
	case "set", "add":
		if len(args) < 3 {
			return current, "", errHeaderUsage
		}
		k := strings.TrimSuffix(args[1], ":")
		v := strings.Join(args[2:], " ")
		if strings.ToLower(args[0]) == "set" {
			h.Set(k, v)
		} else {
			h.Add(k, v)
		}
		return h, fmt.Sprintf("%s: %s", http.CanonicalHeaderKey(k), maskHeaderValue(k, v)), nil
	case "rm", "remove", "unset", "del":
		if len(args) != 2 {
			return current, "", errHeaderUsage
		}
		k := http.CanonicalHeaderKey(strings.TrimSuffix(args[1], ":"))
		if _, ok := h[k]; !ok {
			return current, "", fmt.Errorf("header %q is not set", k)
		}
		h.Del(k)
		return h, "removed " + k, nil
	case "clear":
		return http.Header{}, "cleared all headers", nil
	default:
		return current, "", errHeaderUsage
	}
}

func formatHeaders(h http.Header) string {
	if len(h) == 0 {
		return "no headers (use `header set <Key> <value>`)"
	}
	keys := headerKeys(h)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("\n")
		}
		masked := make([]string, 0, len(h[k]))
		for _, v := range h[k] {
			masked = append(masked, maskHeaderValue(k, v))
		}
		fmt.Fprintf(&b, "%s: %s", k, strings.Join(masked, ", "))
	}
	return b.String()
}

func formatResponseMetadata(headers, trailers http.Header) string {
	var b strings.Builder
	for i, section := range []struct {
		name string
		h    http.Header
	}{{"headers", headers}, {"trailers", trailers}} {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(section.name)
		if len(section.h) == 0 {
			b.WriteString("\n  (none)")
			continue
		}
		for _, line := range strings.Split(formatHeaders(section.h), "\n") {
			b.WriteString("\n  " + line)
		}
	}
	return b.String()
}

func headerKeys(h http.Header) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isSensitiveHeader(k string) bool {
	lk := strings.ToLower(k)
	switch lk {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}
	for _, word := range []string{"auth", "token", "secret", "password", "key", "session", "cookie"} {
		if strings.Contains(lk, word) {
			return true
		}
	}
	return false
}

func maskHeaderValue(k, v string) string {
	if !isSensitiveHeader(k) {
		return v
	}
	r := []rune(v)
	if len(r) <= 8 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:4]) + "…" + fmt.Sprintf("(%d chars)", len(r))
}

func maskCommandEcho(input string) string {
	fields := strings.Fields(input)
	if len(fields) < 4 {
		return input
	}
	cmd, sub, k := strings.ToLower(fields[0]), strings.ToLower(fields[1]), strings.TrimSuffix(fields[2], ":")
	if (cmd != "header" && cmd != "headers") || (sub != "set" && sub != "add") || !isSensitiveHeader(k) {
		return input
	}
	return strings.Join(fields[:3], " ") + " " + maskHeaderValue(k, strings.Join(fields[3:], " "))
}

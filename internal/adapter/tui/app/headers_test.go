package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestApplyHeaderCommand(t *testing.T) {
	h := http.Header{}
	var err error

	h, note, err := applyHeaderCommand(h, []string{"set", "authorization:", "Bearer", "abcdefghijkl"})
	if err != nil || h.Get("Authorization") != "Bearer abcdefghijkl" {
		t.Fatalf("set: h = %v, err = %v", h, err)
	}
	if strings.Contains(note, "abcdefghijkl") {
		t.Errorf("note leaks the secret: %q", note)
	}

	h, _, _ = applyHeaderCommand(h, []string{"add", "X-Tag", "a"})
	h, _, _ = applyHeaderCommand(h, []string{"add", "x-tag", "b"})
	if got := h.Values("X-Tag"); strings.Join(got, ",") != "a,b" {
		t.Errorf("add: %v", got)
	}
	h, _, _ = applyHeaderCommand(h, []string{"set", "X-Tag", "c"})
	if got := h.Values("X-Tag"); strings.Join(got, ",") != "c" {
		t.Errorf("set should replace: %v", got)
	}

	before := h.Clone()
	h2, _, err := applyHeaderCommand(h, []string{"rm", "Nope"})
	if err == nil || len(h2) != len(before) {
		t.Errorf("rm missing: err = %v", err)
	}
	h, _, err = applyHeaderCommand(h, []string{"rm", "x-tag"})
	if err != nil || h.Get("X-Tag") != "" {
		t.Errorf("rm: h = %v, err = %v", h, err)
	}

	for _, bad := range [][]string{{}, {"set", "OnlyKey"}, {"rm"}, {"frobnicate", "x"}} {
		if _, _, err := applyHeaderCommand(h, bad); err == nil {
			t.Errorf("%v should be a usage error", bad)
		}
	}

	h, _, _ = applyHeaderCommand(h, []string{"clear"})
	if len(h) != 0 {
		t.Errorf("clear: %v", h)
	}
}

func TestApplyHeaderCommandDoesNotMutateInput(t *testing.T) {
	orig := http.Header{"A": {"1"}}
	_, _, _ = applyHeaderCommand(orig, []string{"set", "A", "2"})
	if orig.Get("A") != "1" {
		t.Error("input header was mutated")
	}
}

func TestFormatHeadersMasksSensitiveValues(t *testing.T) {
	out := formatHeaders(http.Header{
		"Authorization": {"Bearer abcdefghijklmnop"},
		"X-Api-Key":     {"short"},
		"Cookie":        {"session=abcdefghijk"},
		"X-Trace":       {"visible"},
	})
	for _, leak := range []string{"abcdefghijklmnop", "short", "session=abcdefghijk"} {
		if strings.Contains(out, leak) {
			t.Errorf("output leaks %q:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "X-Trace: visible") || !strings.Contains(out, "X-Api-Key: *****") {
		t.Errorf("output:\n%s", out)
	}
	if lines := strings.Split(out, "\n"); !strings.HasPrefix(lines[0], "Authorization") || !strings.HasPrefix(lines[3], "X-Trace") {
		t.Errorf("headers should be sorted:\n%s", out)
	}
	if formatHeaders(nil) == "" {
		t.Error("empty headers should print a hint")
	}
}

func TestIsSensitiveHeader(t *testing.T) {
	for _, k := range []string{"Authorization", "x-custom-authorization", "X-Auth-Token", "X-Api-Key", "Cookie", "X-Session-Id", "X-Access-Token"} {
		if !isSensitiveHeader(k) {
			t.Errorf("%s should be treated as sensitive", k)
		}
	}
	for _, k := range []string{"User-Agent", "X-Trace", "Accept"} {
		if isSensitiveHeader(k) {
			t.Errorf("%s should not be masked", k)
		}
	}
}

func TestMaskCommandEcho(t *testing.T) {
	tests := map[string]string{
		"header set Authorization Bearer abcdefghijkl": "header set Authorization Bear…(19 chars)",
		"header add x-api-key: xyz":                    "header add x-api-key: ***",
		"header set X-Trace visible":                   "header set X-Trace visible",
		"call CreateUser":                              "call CreateUser",
	}
	for in, want := range tests {
		if got := maskCommandEcho(in); got != want {
			t.Errorf("maskCommandEcho(%q) = %q, want %q", in, got, want)
		}
	}
}

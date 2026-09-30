package bootstrap

import (
	"fmt"
	"net/http"
	"strings"
)

type Config struct {
	SchemaPaths []string
	ImportPaths []string
	Reflection  bool
	Target      string
	Protocol    string
	JSONCodec   bool
	Headers     []string
	StateDir    string

	Profile        string
	ProfileHeaders map[string]string
}

func (c Config) HTTPHeaders() (http.Header, error) {
	h := http.Header{}
	for k, v := range c.ProfileHeaders {
		h.Set(k, v)
	}
	overridden := map[string]bool{}
	for _, raw := range c.Headers {
		k, v, ok := strings.Cut(raw, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header %q (want 'Key: Value')", raw)
		}
		key := http.CanonicalHeaderKey(strings.TrimSpace(k))
		if !overridden[key] {
			h.Del(key)
			overridden[key] = true
		}
		h.Add(key, strings.TrimSpace(v))
	}
	return h, nil
}

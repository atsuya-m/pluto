package bootstrap

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func env(vars map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := vars[k]
		return v, ok
	}
}

func TestFindConfigFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if got, err := FindConfigFile("", nested); err != nil || strings.HasPrefix(got, root) {
		t.Fatalf("no config: got %q, err %v", got, err)
	}

	writeFile(t, filepath.Join(root, ".pluto.yaml"), "profiles: {}\n")
	if got, _ := FindConfigFile("", nested); got != filepath.Join(root, ".pluto.yaml") {
		t.Errorf("walk up: got %q", got)
	}

	writeFile(t, filepath.Join(root, "a", ".pluto.yaml"), "profiles: {}\n")
	if got, _ := FindConfigFile("", nested); got != filepath.Join(root, "a", ".pluto.yaml") {
		t.Errorf("nearest wins: got %q", got)
	}

	explicit := filepath.Join(root, "custom.yaml")
	writeFile(t, explicit, "profiles: {}\n")
	if got, _ := FindConfigFile(explicit, nested); got != explicit {
		t.Errorf("explicit: got %q", got)
	}
	if _, err := FindConfigFile(filepath.Join(root, "missing.yaml"), nested); err == nil {
		t.Error("missing explicit config should fail")
	}
}

func TestLoadConfigFile_Errors(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{
		"unknown key":       "profiles:\n  dev:\n    targte: http://x\n",
		"bad default":       "default_profile: nope\nprofiles:\n  dev: {}\n",
		"bad schema type":   "profiles:\n  dev:\n    schema: {a: b}\n",
		"invalid yaml":      "profiles: [\n",
		"unknown top level": "profile: {}\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".yaml")
			writeFile(t, path, content)
			if _, err := LoadConfigFile(path); err == nil {
				t.Error("expected error")
			}
		})
	}

	empty := filepath.Join(dir, "empty.yaml")
	writeFile(t, empty, "")
	if f, err := LoadConfigFile(empty); err != nil || len(f.ProfileNames()) != 0 {
		t.Errorf("empty file: %v, %v", f, err)
	}
}

func TestProfileExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf", ".pluto.yaml")
	writeFile(t, path, `
default_profile: dev
profiles:
  dev:
    schema: api/api.proto
    import_paths:
      - .
      - ~/googleapis
      - ${PROTO_ROOT}/extra
    target: https://${HOST}
    protocol: grpc
    reflection: false
    json_codec: true
    timeout: 30s
    headers:
      x-api-key: ${API_KEY}
      User-Agent: my-client/1.0.0
  local:
    reflection: true
    target: localhost:8080
`)
	f, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.DefaultProfile != "dev" || !slices.Equal(f.ProfileNames(), []string{"dev", "local"}) {
		t.Errorf("default = %q, names = %v", f.DefaultProfile, f.ProfileNames())
	}

	vars := map[string]string{"HOME": "/home/me", "PROTO_ROOT": "/abs", "HOST": "api.example.com", "API_KEY": "k"}
	p, err := f.Profile("dev", env(vars))
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "conf")
	if !slices.Equal(p.Schema, []string{filepath.Join(base, "api/api.proto")}) {
		t.Errorf("schema = %v", p.Schema)
	}
	if !slices.Equal(p.ImportPaths, []string{base, "/home/me/googleapis", "/abs/extra"}) {
		t.Errorf("import paths = %v", p.ImportPaths)
	}
	if p.Target != "https://api.example.com" || p.Protocol != "grpc" || *p.JSONCodec != true || *p.Reflection {
		t.Errorf("profile = %+v", p)
	}
	if p.Timeout == nil || *p.Timeout != 30*time.Second {
		t.Errorf("timeout = %v", p.Timeout)
	}
	if p.Headers["x-api-key"] != "k" || p.Headers["User-Agent"] != "my-client/1.0.0" {
		t.Errorf("headers = %v", p.Headers)
	}

	vars["API_KEY"] = ""
	delete(vars, "HOST")
	_, err = f.Profile("dev", env(vars))
	if err == nil || !strings.Contains(err.Error(), "API_KEY, HOST") {
		t.Errorf("missing env error = %v", err)
	}

	if _, err := f.Profile("prod", env(vars)); err == nil || !strings.Contains(err.Error(), "available: dev, local") {
		t.Errorf("unknown profile error = %v", err)
	}

	writeFile(t, path, "profiles:\n  dev:\n    timeout: soon\n")
	f, err = LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Profile("dev", env(vars)); err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Errorf("invalid timeout error = %v", err)
	}
}

func TestProfileSummaryDoesNotExpand(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".pluto.yaml")
	writeFile(t, path, "profiles:\n  dev:\n    headers:\n      x-api-key: ${API_KEY}\n")
	f, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := f.Summary("dev"); !ok || s.Headers["x-api-key"] != "${API_KEY}" {
		t.Errorf("summary should not expand values: %+v", s)
	}
}

func TestWithProfilePrecedence(t *testing.T) {
	yes, no := true, false
	timeout := 5 * time.Second
	p := Profile{
		Timeout:     &timeout,
		Name:        "dev",
		Schema:      []string{"/p/api.proto"},
		ImportPaths: []string{"/p"},
		Reflection:  &yes,
		Target:      "https://dev",
		Protocol:    "grpc",
		JSONCodec:   &no,
		Headers:     map[string]string{"X-A": "1"},
	}
	base := Config{SchemaPaths: []string{"."}, Target: "http://localhost:8080", Protocol: "connect"}

	c := base.WithProfile(p, func(string) bool { return false })
	if c.Profile != "dev" || c.Target != "https://dev" || c.Protocol != "grpc" || !c.Reflection || c.SchemaPaths[0] != "/p/api.proto" || c.Timeout != timeout {
		t.Errorf("profile not applied: %+v", c)
	}

	explicit := base
	explicit.Target = "http://override"
	explicit.Timeout = time.Minute
	c = explicit.WithProfile(p, func(f string) bool { return f == "target" || f == "timeout" })
	if c.Target != "http://override" || c.Protocol != "grpc" || c.Timeout != time.Minute {
		t.Errorf("explicit flag should win: %+v", c)
	}
}

func TestHTTPHeadersMergesProfileAndFlags(t *testing.T) {
	c := Config{
		ProfileHeaders: map[string]string{"User-Agent": "profile", "X-Api-Key": "k"},
		Headers:        []string{"user-agent: flag", "X-Tag: a", "X-Tag: b"},
	}
	h, err := c.HTTPHeaders()
	if err != nil {
		t.Fatal(err)
	}
	if h.Get("User-Agent") != "flag" || len(h.Values("User-Agent")) != 1 {
		t.Errorf("flag should override profile: %v", h.Values("User-Agent"))
	}
	if h.Get("X-Api-Key") != "k" || strings.Join(h.Values("X-Tag"), ",") != "a,b" {
		t.Errorf("headers = %v", h)
	}
}

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atsuya-m/pluto/internal/testutil/testserver"
)

var update = flag.Bool("update", false, "update golden files")

var schemaDir = filepath.Join("..", "..", "..", "testdata", "proto")

type result struct {
	code   int
	stdout string
	stderr string
}

func pluto(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &stdout, &stderr)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s (run with -update to accept)\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

func TestDesc_Golden(t *testing.T) {
	tests := []struct {
		golden string
		args   []string
	}{
		{"desc_services.txt", []string{"desc", "services"}},
		{"desc_services.json", []string{"desc", "services", "-o", "json"}},
		{"desc_rpcs.txt", []string{"desc", "rpcs"}},
		{"desc_rpcs.json", []string{"desc", "rpcs", "-o", "json"}},
		{"desc_rpcs_admin.json", []string{"desc", "rpcs", "AdminService", "-o", "json"}},
		{"desc_rpc_create_user.txt", []string{"desc", "rpc", "CreateUser"}},
		{"desc_rpc_create_user.json", []string{"desc", "rpc", "CreateUser", "-o", "json"}},
		{"desc_message_profile.json", []string{"desc", "message", "Profile", "-o", "json"}},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			r := pluto(t, "", append([]string{"--schema", schemaDir}, tt.args...)...)
			if r.code != exitOK || r.stderr != "" {
				t.Fatalf("code = %d, stderr = %s", r.code, r.stderr)
			}
			if strings.Contains(r.stdout, "\x1b[") {
				t.Error("output must not contain ANSI escape sequences")
			}
			golden(t, tt.golden, r.stdout)
		})
	}
}

func TestDesc_JSONIsDeterministic(t *testing.T) {
	first := pluto(t, "", "--schema", schemaDir, "desc", "rpcs", "-o", "json").stdout
	for range 5 {
		if got := pluto(t, "", "--schema", schemaDir, "desc", "rpcs", "-o", "json").stdout; got != first {
			t.Fatal("desc rpcs -o json is not deterministic")
		}
	}
	if !json.Valid([]byte(first)) {
		t.Error("output is not valid JSON")
	}
}

func TestDesc_Errors(t *testing.T) {
	ambiguous := pluto(t, "", "--schema", schemaDir, "desc", "rpc", "GetUser", "-o", "json")
	if ambiguous.code != exitNotFound {
		t.Errorf("ambiguous code = %d, want %d", ambiguous.code, exitNotFound)
	}
	golden(t, "error_ambiguous.json", ambiguous.stdout)

	notFound := pluto(t, "", "--schema", schemaDir, "desc", "rpc", "Nope")
	if notFound.code != exitNotFound || notFound.stdout != "" || !strings.Contains(notFound.stderr, `rpc "Nope" not found`) {
		t.Errorf("not found = %+v", notFound)
	}

	usage := pluto(t, "", "--schema", schemaDir, "desc", "rpc")
	if usage.code != exitUsage {
		t.Errorf("usage code = %d, want %d", usage.code, exitUsage)
	}

	badOutput := pluto(t, "", "--schema", schemaDir, "desc", "rpcs", "-o", "yaml")
	if badOutput.code != exitUsage || !strings.Contains(badOutput.stderr, "unknown output format") {
		t.Errorf("bad output = %+v", badOutput)
	}

	badSchema := pluto(t, "", "--schema", "does-not-exist", "desc", "rpcs")
	if badSchema.code != exitError {
		t.Errorf("bad schema code = %d, want %d", badSchema.code, exitError)
	}
}

func TestCall(t *testing.T) {
	srv := testserver.New(t)
	base := []string{"--schema", schemaDir, "--target", srv.URL}

	t.Run("json output", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "CreateUser", "-d", `{"name":"Taro"}`, "-o", "json", "-H", "Authorization: Bearer x")...)
		if r.code != exitOK {
			t.Fatalf("code = %d, stderr = %s", r.code, r.stderr)
		}
		var out struct {
			Message struct {
				User struct{ ID, Name string }
			}
			Headers  map[string][]string
			Trailers map[string][]string
		}
		if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, r.stdout)
		}
		if out.Message.User.ID != "u-1" || out.Message.User.Name != "Taro" || out.Headers["X-Test"][0] != "1" || out.Trailers["X-Trailer"][0] != "2" {
			t.Errorf("out = %+v", out)
		}
		if got := (<-srv.Headers).Get("Authorization"); got != "Bearer x" {
			t.Errorf("server saw Authorization = %q", got)
		}
	})

	t.Run("text output from stdin", func(t *testing.T) {
		r := pluto(t, `{"name":"Hanako"}`, append(base, "--protocol", "grpc", "call", "UserService/CreateUser", "-d", "-")...)
		if r.code != exitOK {
			t.Fatalf("code = %d, stderr = %s", r.code, r.stderr)
		}
		want := "{\n  \"user\": {\n    \"id\": \"u-1\",\n    \"name\": \"Hanako\"\n  }\n}\n"
		if r.stdout != want {
			t.Errorf("stdout = %q, want %q", r.stdout, want)
		}
	})

	t.Run("data from file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "req.json")
		if err := os.WriteFile(path, []byte(`{"name":"File"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		r := pluto(t, "", append(base, "call", "CreateUser", "-d", "@"+path)...)
		if r.code != exitOK || !strings.Contains(r.stdout, `"File"`) {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "CreateUser", "-d", `{}`, "-o", "json")...)
		if r.code != exitRPC {
			t.Errorf("code = %d, want %d", r.code, exitRPC)
		}
		golden(t, "error_rpc.json", r.stdout)
	})

	t.Run("rpc error metadata", func(t *testing.T) {
		for _, protocol := range []string{"connect", "grpc"} {
			r := pluto(t, "", append(base, "--protocol", protocol, "call", "CreateUser", "-d", `{"name":"not-found"}`, "-o", "json")...)
			<-srv.Headers
			var out struct {
				Error struct {
					Code     string
					Metadata map[string][]string
				}
			}
			if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
				t.Fatalf("%s: %v\n%s", protocol, err, r.stdout)
			}
			if r.code != exitRPC || out.Error.Code != "not_found" || strings.Join(out.Error.Metadata["X-Error-Reason"], ",") != "1001" {
				t.Errorf("%s: code = %d, error = %+v", protocol, r.code, out.Error)
			}
			for k := range out.Error.Metadata {
				if k == "Date" || k == "Content-Type" || strings.HasPrefix(k, "Grpc-") {
					t.Errorf("%s: transport header %s should be filtered", protocol, k)
				}
			}
		}
		r := pluto(t, "", append(base, "call", "CreateUser", "-d", `{"name":"not-found"}`)...)
		<-srv.Headers
		if !strings.Contains(r.stderr, "metadata: X-Error-Reason: 1001") {
			t.Errorf("text error should show metadata:\n%s", r.stderr)
		}
	})

	t.Run("invalid request json", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "CreateUser", "-d", `{"nope":1}`)...)
		if r.code != exitError || !strings.Contains(r.stderr, "decode request json") {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("reflection", func(t *testing.T) {
		r := pluto(t, "", "--reflection", "--target", srv.URL, "call", "CreateUser", "-d", `{"name":"Reflect"}`)
		if r.code != exitOK || !strings.Contains(r.stdout, `"Reflect"`) {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("server stream text", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "WatchUsers")...)
		<-srv.Headers
		if r.code != exitOK || strings.Count(r.stdout, `"id"`) != 3 {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("server stream ndjson", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "WatchUsers", "-o", "json")...)
		<-srv.Headers
		if r.code != exitOK {
			t.Fatalf("r = %+v", r)
		}
		lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
		if len(lines) != 4 {
			t.Fatalf("lines = %d, want 4\n%s", len(lines), r.stdout)
		}
		for _, l := range lines {
			if !json.Valid([]byte(l)) {
				t.Errorf("invalid JSON line: %s", l)
			}
		}
		if lines[0] != `{"message":{"id":"u-1"}}` {
			t.Errorf("first line = %s", lines[0])
		}
		if !strings.HasPrefix(lines[3], `{"summary":{"count":3,`) || !strings.Contains(lines[3], `"X-Trailer":["done"]`) {
			t.Errorf("summary line = %s", lines[3])
		}
	})

	t.Run("server stream error", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "WatchUsers", "-o", "json", "-H", "X-Fail-After: 2")...)
		<-srv.Headers
		lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
		if r.code != exitRPC || len(lines) != 2 || !strings.HasPrefix(lines[1], `{"error":{"code":"unavailable"`) {
			t.Errorf("code = %d\n%s", r.code, r.stdout)
		}
	})

	t.Run("client streaming", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "ImportUsers", "-d", "{\"name\":\"A\"}\n{\"name\":\"B\"}")...)
		<-srv.Headers
		if r.code != exitOK || r.stdout != "{\n  \"imported\": 2\n}\n" {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("bidi streaming ndjson", func(t *testing.T) {
		r := pluto(t, "", append(base, "--protocol", "grpc", "call", "Chat", "-d", `[{"text":"a"},{"text":"b"}]`, "-o", "json")...)
		<-srv.Headers
		lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
		if r.code != exitOK || len(lines) != 3 {
			t.Fatalf("r = %+v", r)
		}
		if lines[0] != `{"message":{"text":"echo: a"}}` || lines[1] != `{"message":{"text":"echo: b"}}` {
			t.Errorf("lines = %v", lines)
		}
		if !strings.HasPrefix(lines[2], `{"summary":{`) || !strings.Contains(lines[2], `"received":2`) || !strings.Contains(lines[2], `"sent":2`) {
			t.Errorf("summary = %s", lines[2])
		}
	})

	t.Run("client streaming error", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "ImportUsers", "-d", `{"name":""}`, "-o", "json")...)
		<-srv.Headers
		if r.code != exitRPC || !strings.HasPrefix(r.stdout, `{"error":{"code":"invalid_argument"`) {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("streaming without data", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "Chat")...)
		if r.code != exitUsage || !strings.Contains(r.stderr, "pass one or more request messages") {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("invalid header", func(t *testing.T) {
		r := pluto(t, "", append(base, "call", "CreateUser", "-H", "no-colon")...)
		if r.code != exitUsage || !strings.Contains(r.stderr, "invalid header") {
			t.Errorf("r = %+v", r)
		}
	})
}

func TestProfiles(t *testing.T) {
	srv := testserver.New(t)
	dir := t.TempDir()
	schema, err := filepath.Abs(schemaDir)
	if err != nil {
		t.Fatal(err)
	}
	config := `
default_profile: test
profiles:
  test:
    schema: ` + schema + `
    target: ` + srv.URL + `
    protocol: grpc
    headers:
      X-Api-Key: ${TEST_API_KEY}
  refl:
    reflection: true
    target: ` + srv.URL + `
`
	if err := os.WriteFile(filepath.Join(dir, ".pluto.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"TEST_API_KEY": "dummy-value"}
	run := func(args ...string) result {
		var stdout, stderr bytes.Buffer
		getenv := func(k string) (string, bool) { v, ok := vars[k]; return v, ok }
		code := runWithEnv(context.Background(), args, strings.NewReader(""), &stdout, &stderr, getenv, func() (string, error) { return dir, nil })
		return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
	}

	t.Run("default profile", func(t *testing.T) {
		r := run("call", "CreateUser", "-d", `{"name":"Profile"}`)
		h := <-srv.Headers
		if r.code != exitOK || !strings.Contains(r.stdout, `"Profile"`) {
			t.Fatalf("r = %+v", r)
		}
		if h.Get("X-Api-Key") != "dummy-value" {
			t.Errorf("profile header not sent: %v", h)
		}
	})

	t.Run("select profile", func(t *testing.T) {
		r := run("-p", "refl", "desc", "services")
		if r.code != exitOK || !strings.Contains(r.stdout, "user.v1.UserService") {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("flag overrides profile", func(t *testing.T) {
		r := run("-t", "http://127.0.0.1:1", "call", "CreateUser", "-d", `{"name":"x"}`)
		if r.code != exitRPC || !strings.Contains(r.stderr, "unavailable") {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("header flag overrides profile header", func(t *testing.T) {
		r := run("-H", "X-Api-Key: other", "call", "CreateUser", "-d", `{"name":"x"}`)
		h := <-srv.Headers
		if r.code != exitOK || h.Get("X-Api-Key") != "other" || len(h.Values("X-Api-Key")) != 1 {
			t.Errorf("r = %+v, header = %v", r, h.Values("X-Api-Key"))
		}
	})

	t.Run("missing env var", func(t *testing.T) {
		delete(vars, "TEST_API_KEY")
		defer func() { vars["TEST_API_KEY"] = "dummy-value" }()
		r := run("desc", "services")
		if r.code != exitUsage || !strings.Contains(r.stderr, "TEST_API_KEY") {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("unknown profile", func(t *testing.T) {
		r := run("-p", "nope", "desc", "services")
		if r.code != exitUsage || !strings.Contains(r.stderr, `profile "nope" is not defined`) {
			t.Errorf("r = %+v", r)
		}
	})

	t.Run("profiles command hides header values", func(t *testing.T) {
		r := run("profiles")
		if r.code != exitOK || !strings.Contains(r.stdout, "* test") || !strings.Contains(r.stdout, "headers: X-Api-Key") || strings.Contains(r.stdout, "dummy-value") || strings.Contains(r.stdout, "${TEST_API_KEY}") {
			t.Errorf("r = %+v", r)
		}
		j := run("profiles", "-o", "json")
		var out struct {
			Config   string
			Profiles []struct {
				Name    string
				Default bool
				Headers []string
			}
		}
		if err := json.Unmarshal([]byte(j.stdout), &out); err != nil || len(out.Profiles) != 2 || out.Profiles[0].Name != "refl" || !out.Profiles[1].Default {
			t.Errorf("json = %s, err = %v", j.stdout, err)
		}
	})

	t.Run("profile without config", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		empty := t.TempDir()
		code := runWithEnv(context.Background(), []string{"-p", "x", "desc", "services"}, strings.NewReader(""), &stdout, &stderr,
			func(string) (string, bool) { return "", false }, func() (string, error) { return empty, nil })
		if code != exitUsage || !strings.Contains(stderr.String(), "no config file was found") {
			t.Errorf("code = %d, stderr = %s", code, stderr.String())
		}
	})
}

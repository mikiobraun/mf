package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mikiobraun/mf/registry"
)

// scriptingStore holds a keyed entry and an unkeyed one on separate endpoints.
func scriptingStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s, err := registry.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.PutEndpoint(registry.Endpoint{Name: "gw", URL: "https://gw.example/v1"})
	s.PutEndpoint(registry.Endpoint{Name: "local", URL: "http://localhost:11434/v1"})
	s.PutModel(registry.Model{Name: "fast", Endpoint: "gw", Model: "gemini/flash", Tags: []string{"cheap"}})
	s.PutModel(registry.Model{Name: "llama", Endpoint: "local", Model: "llama3"})
	s.PutSecret("gw", "", "sk-secret")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSecrets(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestGetJSONPrintsTheEntryWithoutTheKey(t *testing.T) {
	dir := scriptingStore(t)
	out, err := runIn(t, "--config", dir, "get", "fast", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "sk-secret") {
		t.Fatalf("key leaked into JSON:\n%s", out)
	}
	var got entryJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	want := entryJSON{Name: "fast", Endpoint: "gw", URL: "https://gw.example/v1", API: "openai",
		Model: "gemini/flash", Tags: []string{"cheap"}, HasKey: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetNeedsExactlyOneOfFieldAndJSON(t *testing.T) {
	dir := scriptingStore(t)
	if _, err := runIn(t, "--config", dir, "get", "fast"); err == nil || !strings.Contains(err.Error(), "--json") {
		t.Errorf("no field, no --json: got %v", err)
	}
	if _, err := runIn(t, "--config", dir, "get", "fast", "url", "--json"); err == nil || !strings.Contains(err.Error(), "drop") {
		t.Errorf("field and --json: got %v", err)
	}
}

func TestListJSONIsAnArrayWithEmptyTagsAsArrays(t *testing.T) {
	dir := scriptingStore(t)
	out, err := runIn(t, "--config", dir, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "sk-secret") || strings.Contains(out, "null") {
		t.Fatalf("unexpected content:\n%s", out)
	}
	var got []entryJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	if len(got) != 2 || got[0].Name != "fast" || !got[0].HasKey || got[1].Name != "llama" || got[1].HasKey {
		t.Errorf("got %+v", got)
	}

	out, err = runIn(t, "--config", dir, "list", "--json", "--tags", "nothing")
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("empty filter: got %q, %v", out, err)
	}
}

// captureExec swaps execve for a recorder, so exec tests run without being
// replaced by the command.
func captureExec(t *testing.T) *struct{ argv, env []string } {
	t.Helper()
	got := &struct{ argv, env []string }{}
	orig := execve
	execve = func(_ string, argv, env []string) error {
		got.argv, got.env = argv, env
		return nil
	}
	t.Cleanup(func() { execve = orig })
	return got
}

func envValue(env []string, name string) (string, bool) {
	var val string
	var found bool
	for _, kv := range env {
		if n, v, _ := strings.Cut(kv, "="); n == name {
			val, found = v, true
		}
	}
	return val, found
}

func TestExecSetsTheModelVariables(t *testing.T) {
	dir := scriptingStore(t)
	t.Setenv("OPENAI_MODEL", "stale")
	got := captureExec(t)
	if _, err := runIn(t, "--config", dir, "exec", "fast", "--", "true", "--flag"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.argv, []string{"true", "--flag"}) {
		t.Errorf("argv = %v", got.argv)
	}
	for name, want := range map[string]string{
		"OPENAI_BASE_URL": "https://gw.example/v1",
		"OPENAI_MODEL":    "gemini/flash",
		"OPENAI_API_KEY":  "sk-secret",
	} {
		if v, _ := envValue(got.env, name); v != want {
			t.Errorf("%s = %q, want %q", name, v, want)
		}
	}
	var models int
	for _, kv := range got.env {
		if strings.HasPrefix(kv, "OPENAI_MODEL=") {
			models++
		}
	}
	if models != 1 {
		t.Errorf("OPENAI_MODEL appears %d times; the inherited value should be dropped", models)
	}
}

func TestExecRemovesAnInheritedKeyWhenNoneIsConfigured(t *testing.T) {
	dir := scriptingStore(t)
	t.Setenv("OPENAI_API_KEY", "sk-from-the-shell")
	got := captureExec(t)
	if _, err := runIn(t, "--config", dir, "exec", "llama", "--", "true"); err != nil {
		t.Fatal(err)
	}
	if v, found := envValue(got.env, "OPENAI_API_KEY"); found {
		t.Errorf("OPENAI_API_KEY = %q, want it unset", v)
	}
}

func TestExecPrefixReplacesOpenAI(t *testing.T) {
	dir := scriptingStore(t)
	got := captureExec(t)
	if _, err := runIn(t, "--config", dir, "exec", "--prefix", "SMOKE_", "fast", "--", "true"); err != nil {
		t.Fatal(err)
	}
	if v, _ := envValue(got.env, "SMOKE_API_KEY"); v != "sk-secret" {
		t.Errorf("SMOKE_API_KEY = %q", v)
	}
	if _, found := envValue(got.env, "OPENAI_API_KEY"); found {
		t.Error("OPENAI_API_KEY set despite --prefix")
	}
}

func TestExecNeedsTheDash(t *testing.T) {
	dir := scriptingStore(t)
	captureExec(t)
	for _, args := range [][]string{
		{"exec", "fast", "true"},
		{"exec", "fast", "extra", "--", "true"},
	} {
		_, err := runIn(t, append([]string{"--config", dir}, args...)...)
		if err == nil || !strings.Contains(err.Error(), "then -- and the command") {
			t.Errorf("%v: got %v", args, err)
		}
	}
}

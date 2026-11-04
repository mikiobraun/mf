package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMissingDirIsEmpty(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Open on missing dir: %v", err)
	}
	if len(s.Config.Endpoints) != 0 || len(s.Config.Models) != 0 {
		t.Fatalf("expected empty store, got %+v", s.Config)
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.PutEndpoint(Endpoint{Name: "local", URL: "http://localhost:11434"})
	s.PutEndpoint(Endpoint{Name: "anthropic", URL: "https://api.anthropic.com", API: APIAnthropic})
	s.PutModel(Model{Name: "fast", Endpoint: "local", Model: "qwen2.5-coder", Tags: []string{"coding", "local"}})
	s.PutSecret("local", "", "endpoint-key")
	s.PutSecret("local", "fast", "model-key")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSecrets(); err != nil {
		t.Fatal(err)
	}

	got, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Config.Endpoints) != 2 || len(got.Config.Models) != 1 || len(got.Secrets.Secrets) != 2 {
		t.Fatalf("round trip lost entries: %+v %+v", got.Config, got.Secrets)
	}
	ep, ok := got.Endpoint("anthropic")
	if !ok || ep.Dialect() != APIAnthropic {
		t.Fatalf("endpoint dialect not preserved: %+v", ep)
	}
	local, _ := got.Endpoint("local")
	if local.Dialect() != APIOpenAI {
		t.Errorf("empty api should default to %q, got %q", APIOpenAI, local.Dialect())
	}
	m, ok := got.Model("fast")
	if !ok || m.Model != "qwen2.5-coder" || !m.HasTag("coding") {
		t.Fatalf("model not preserved: %+v", m)
	}
}

func TestSecretsFilePermissions(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.PutSecret("local", "", "k")
	if err := s.SaveSecrets(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.SecretsPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("models.secrets perm = %o, want 600", perm)
	}
}

func TestRemoveEndpointCascades(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.PutEndpoint(Endpoint{Name: "a", URL: "http://a"})
	s.PutEndpoint(Endpoint{Name: "b", URL: "http://b"})
	s.PutModel(Model{Name: "m1", Endpoint: "a", Model: "x"})
	s.PutModel(Model{Name: "m2", Endpoint: "a", Model: "y"})
	s.PutModel(Model{Name: "keep", Endpoint: "b", Model: "z"})
	s.PutSecret("a", "", "ka")
	s.PutSecret("b", "", "kb")

	n, found := s.RemoveEndpoint("a")
	if !found || n != 2 {
		t.Fatalf("RemoveEndpoint = %d, %v; want 2, true", n, found)
	}
	if _, ok := s.Model("keep"); !ok {
		t.Error("removed a model belonging to another endpoint")
	}
	if _, ok := s.Model("m1"); ok {
		t.Error("m1 should be gone")
	}
	if len(s.Secrets.Secrets) != 1 || s.Secrets.Secrets[0].Endpoint != "b" {
		t.Errorf("secrets not cascaded: %+v", s.Secrets.Secrets)
	}
}

func TestRemoveModelDropsModelScopedSecretOnly(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.PutEndpoint(Endpoint{Name: "a", URL: "http://a"})
	s.PutModel(Model{Name: "m", Endpoint: "a", Model: "x"})
	s.PutSecret("a", "", "endpoint-key")
	s.PutSecret("a", "m", "model-key")

	if !s.RemoveModel("m") {
		t.Fatal("RemoveModel returned false")
	}
	if len(s.Secrets.Secrets) != 1 || s.Secrets.Secrets[0].Model != "" {
		t.Errorf("expected only the endpoint secret to remain, got %+v", s.Secrets.Secrets)
	}
}

func TestPutIsUpsert(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.PutEndpoint(Endpoint{Name: "a", URL: "http://a"})
	s.PutEndpoint(Endpoint{Name: "a", URL: "http://changed"})
	if len(s.Config.Endpoints) != 1 {
		t.Fatalf("PutEndpoint duplicated: %+v", s.Config.Endpoints)
	}
	if ep, _ := s.Endpoint("a"); ep.URL != "http://changed" {
		t.Errorf("URL = %q, want http://changed", ep.URL)
	}
	s.PutSecret("a", "", "one")
	s.PutSecret("a", "", "two")
	if len(s.Secrets.Secrets) != 1 || s.Secrets.Secrets[0].Key != "two" {
		t.Errorf("PutSecret did not replace: %+v", s.Secrets.Secrets)
	}
}

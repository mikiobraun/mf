package registry

import (
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.PutEndpoint(Endpoint{Name: "anthropic", URL: "https://api.anthropic.com"})
	s.PutEndpoint(Endpoint{Name: "bifrost", URL: "https://bifrost.example/v1"})
	s.PutModel(Model{Name: "fast", Endpoint: "anthropic", Model: "claude-haiku-4-5", Tags: []string{"coding", "cheap"}})
	s.PutModel(Model{Name: "smart", Endpoint: "anthropic", Model: "claude-opus-4-6", Tags: []string{"research", "coding"}})
	return s
}

func TestResolveAlias(t *testing.T) {
	s := testStore(t)
	ref, err := Resolve(s, "fast")
	if err != nil {
		t.Fatal(err)
	}
	if ref.ModelID != "claude-haiku-4-5" || ref.Endpoint.Name != "anthropic" || !ref.Entry {
		t.Fatalf("got %+v", ref)
	}
}

func TestResolveEndpointSlashModel(t *testing.T) {
	s := testStore(t)
	// The model ID itself contains slashes; only the first one separates
	// the endpoint name.
	ref, err := Resolve(s, "bifrost/gemini/gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}
	if ref.ModelID != "gemini/gemini-2.5-flash" {
		t.Errorf("ModelID = %q, want gemini/gemini-2.5-flash", ref.ModelID)
	}
	if ref.Endpoint.Name != "bifrost" {
		t.Errorf("Endpoint = %q, want bifrost", ref.Endpoint.Name)
	}
	if ref.Entry {
		t.Error("direct reference should not be marked as a stored entry")
	}
}

func TestResolveTagPicksFirstAndReportsRest(t *testing.T) {
	s := testStore(t)
	ref, err := Resolve(s, "@coding")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Name != "fast" {
		t.Errorf("Name = %q, want fast (first in file order)", ref.Name)
	}
	if len(ref.TagMatches) != 1 || ref.TagMatches[0] != "smart" {
		t.Errorf("TagMatches = %v, want [smart]", ref.TagMatches)
	}
}

func TestResolveErrors(t *testing.T) {
	s := testStore(t)
	for _, addr := range []string{"", "nope", "@nosuchtag", "unknownep/model", "bifrost/"} {
		if _, err := Resolve(s, addr); err == nil {
			t.Errorf("Resolve(%q) succeeded, want error", addr)
		}
	}
}

func TestAliasWinsOverEndpointSlashModel(t *testing.T) {
	s := testStore(t)
	s.PutModel(Model{Name: "bifrost/weird", Endpoint: "anthropic", Model: "aliased"})
	ref, err := Resolve(s, "bifrost/weird")
	if err != nil {
		t.Fatal(err)
	}
	if ref.ModelID != "aliased" {
		t.Errorf("ModelID = %q, want aliased (the alias should win)", ref.ModelID)
	}
}

func TestKeyPrecedence(t *testing.T) {
	s := testStore(t)
	s.PutSecret("anthropic", "", "endpoint-key")

	ref, _ := Resolve(s, "fast")
	if key, ok := ref.Key(s); !ok || key != "endpoint-key" {
		t.Fatalf("endpoint key: got %q, %v", key, ok)
	}

	// A model-scoped secret beats the endpoint-wide one.
	s.PutSecret("anthropic", "fast", "model-key")
	if key, _ := ref.Key(s); key != "model-key" {
		t.Errorf("model-scoped key should win, got %q", key)
	}

	// The environment beats both.
	t.Setenv("MF_KEY_ANTHROPIC", "env-key")
	if key, _ := ref.Key(s); key != "env-key" {
		t.Errorf("env key should win, got %q", key)
	}
}

func TestDirectRefIgnoresModelScopedSecret(t *testing.T) {
	s := testStore(t)
	s.PutSecret("anthropic", "", "endpoint-key")
	s.PutSecret("anthropic", "fast", "model-key")

	// Addressing by endpoint/model is not the "fast" entry, so the
	// model-scoped secret must not apply.
	ref, err := Resolve(s, "anthropic/claude-haiku-4-5")
	if err != nil {
		t.Fatal(err)
	}
	if key, _ := ref.Key(s); key != "endpoint-key" {
		t.Errorf("got %q, want endpoint-key", key)
	}
}

func TestNoKeyConfigured(t *testing.T) {
	s := testStore(t)
	ref, _ := Resolve(s, "fast")
	if key, ok := ref.Key(s); ok {
		t.Errorf("expected no key, got %q", key)
	}
}

func TestKeyEnvVar(t *testing.T) {
	cases := map[string]string{
		"anthropic":  "MF_KEY_ANTHROPIC",
		"my-gateway": "MF_KEY_MY_GATEWAY",
		"local.1":    "MF_KEY_LOCAL_1",
	}
	for in, want := range cases {
		if got := KeyEnvVar(in); got != want {
			t.Errorf("KeyEnvVar(%q) = %q, want %q", in, got, want)
		}
	}
}

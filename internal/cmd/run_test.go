package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mikiobraun/mf/registry"
)

// withFakeOpencode puts an executable named opencode on PATH, so LookPath
// succeeds without opencode installed; execve is captured and never runs it.
func withFakeOpencode(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

// opencodeContent decodes the config mf passed in OPENCODE_CONFIG_CONTENT.
func opencodeContent(t *testing.T, env []string) map[string]any {
	t.Helper()
	raw, ok := envValue(env, "OPENCODE_CONFIG_CONTENT")
	if !ok {
		t.Fatal("OPENCODE_CONFIG_CONTENT not set")
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("%v:\n%s", err, raw)
	}
	return cfg
}

func TestRunOpencodeConfiguresOnlyTheNamedModels(t *testing.T) {
	dir := scriptingStore(t)
	withEntry(t, dir, registry.Model{Name: "other", Endpoint: "gw", Model: "not/named"})
	withFakeOpencode(t)
	got := captureExec(t)

	if _, err := runIn(t, "--config", dir, "run", "opencode", "fast", "--small", "llama", "--", "run", "hi"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.argv, []string{"opencode", "run", "hi"}) {
		t.Errorf("argv = %v", got.argv)
	}
	raw, _ := envValue(got.env, "OPENCODE_CONFIG_CONTENT")
	if strings.Contains(raw, "sk-secret") {
		t.Fatalf("key written into the config:\n%s", raw)
	}
	if strings.Contains(raw, "not/named") {
		t.Errorf("a model that was not named was configured:\n%s", raw)
	}

	cfg := opencodeContent(t, got.env)
	if cfg["model"] != "gw/gemini/flash" || cfg["small_model"] != "local/llama3" {
		t.Errorf("model = %v, small_model = %v", cfg["model"], cfg["small_model"])
	}
	gw := cfg["provider"].(map[string]any)["gw"].(map[string]any)
	options := gw["options"].(map[string]any)
	if options["baseURL"] != "https://gw.example/v1" || options["apiKey"] != "{env:MF_KEY_GW}" {
		t.Errorf("gw options = %v", options)
	}
	if name := gw["models"].(map[string]any)["gemini/flash"].(map[string]any)["name"]; name != "fast" {
		t.Errorf("display name = %v, want the alias", name)
	}
	if v, _ := envValue(got.env, "MF_KEY_GW"); v != "sk-secret" {
		t.Errorf("MF_KEY_GW = %q", v)
	}
	local := cfg["provider"].(map[string]any)["local"].(map[string]any)
	if _, has := local["options"].(map[string]any)["apiKey"]; has {
		t.Error("keyless endpoint got an apiKey")
	}
}

func TestRunOpencodeRejectsTwoKeysOnOneProvider(t *testing.T) {
	dir := scriptingStore(t)
	withEntry(t, dir, registry.Model{Name: "special", Endpoint: "gw", Model: "other/model"})
	s, err := registry.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.PutSecret("gw", "special", "sk-special")
	if err := s.SaveSecrets(); err != nil {
		t.Fatal(err)
	}
	withFakeOpencode(t)
	captureExec(t)

	_, err = runIn(t, "--config", dir, "run", "opencode", "fast", "special")
	if err == nil || !strings.Contains(err.Error(), "one key per provider") {
		t.Errorf("got %v", err)
	}
}

func TestRunOpencodePassesLimits(t *testing.T) {
	dir := scriptingStore(t)
	for _, args := range [][]string{{"set", "fast", "context", "131072"}, {"set", "fast", "output", "16384"}} {
		if _, err := runIn(t, append([]string{"--config", dir}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	withFakeOpencode(t)
	got := captureExec(t)
	if _, err := runIn(t, "--config", dir, "run", "opencode", "fast"); err != nil {
		t.Fatal(err)
	}
	cfg := opencodeContent(t, got.env)
	model := cfg["provider"].(map[string]any)["gw"].(map[string]any)["models"].(map[string]any)["gemini/flash"].(map[string]any)
	want := map[string]any{"context": float64(131072), "output": float64(16384)}
	if !reflect.DeepEqual(model["limit"], want) {
		t.Errorf("limit = %v, want %v", model["limit"], want)
	}
}

func TestRunOpencodeRejectsAOneSidedLimit(t *testing.T) {
	dir := scriptingStore(t)
	if _, err := runIn(t, "--config", dir, "set", "fast", "output", "16384"); err != nil {
		t.Fatal(err)
	}
	withFakeOpencode(t)
	captureExec(t)
	_, err := runIn(t, "--config", dir, "run", "opencode", "fast")
	if err == nil || !strings.Contains(err.Error(), "mf set fast context") {
		t.Errorf("got %v", err)
	}
}

func TestSetLimitRejectsNonNumbers(t *testing.T) {
	dir := scriptingStore(t)
	for _, v := range []string{"lots", "-1", "16k"} {
		if _, err := runIn(t, "--config", dir, "set", "fast", "context", v); err == nil {
			t.Errorf("context %q accepted", v)
		}
	}
	if _, err := runIn(t, "--config", dir, "set", "fast", "context", "8192"); err != nil {
		t.Fatal(err)
	}
	out, err := runIn(t, "--config", dir, "get", "fast", "context")
	if err != nil || out != "8192\n" {
		t.Errorf("get context = %q, %v", out, err)
	}
	if _, err := runIn(t, "--config", dir, "set", "fast", "context", "0"); err != nil {
		t.Fatal(err)
	}
	out, err = runIn(t, "--config", dir, "get", "fast", "--json")
	if err != nil || strings.Contains(out, "context") {
		t.Errorf("0 should remove the limit: %q, %v", out, err)
	}
}

func TestRunOpencodeNeedsAModelBeforeTheDash(t *testing.T) {
	dir := scriptingStore(t)
	withFakeOpencode(t)
	captureExec(t)
	_, err := runIn(t, "--config", dir, "run", "opencode", "--", "run")
	if err == nil || !strings.Contains(err.Error(), "needs a model before --") {
		t.Errorf("got %v", err)
	}
}

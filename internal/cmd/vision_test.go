package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikiobraun/mf/registry"
)

// vision is set with true or false, read back by get and the registry,
// and written to models.toml only when true.
func TestSetVision(t *testing.T) {
	dir := scriptingStore(t)
	for _, v := range []string{"yes", "1", "True"} {
		if _, err := runIn(t, "--config", dir, "set", "fast", "vision", v); err == nil {
			t.Errorf("vision %q accepted", v)
		}
	}

	out, err := runIn(t, "--config", dir, "get", "fast", "vision")
	if err != nil || out != "false\n" {
		t.Errorf("get vision before set = %q, %v", out, err)
	}

	if _, err := runIn(t, "--config", dir, "set", "fast", "vision", "true"); err != nil {
		t.Fatal(err)
	}
	out, err = runIn(t, "--config", dir, "get", "fast", "vision")
	if err != nil || out != "true\n" {
		t.Errorf("get vision = %q, %v", out, err)
	}
	out, err = runIn(t, "--config", dir, "get", "fast", "--json")
	if err != nil || !strings.Contains(out, `"vision": true`) {
		t.Errorf("get --json = %q, %v", out, err)
	}
	s, err := registry.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := registry.Resolve(s, "fast")
	if err != nil || !ref.Vision {
		t.Errorf("Resolve(fast).Vision = %v, %v", ref.Vision, err)
	}
	if ref, _ := registry.Resolve(s, "gw/gemini/flash"); ref.Vision {
		t.Error("a direct endpoint/model reference claims vision")
	}

	if _, err := runIn(t, "--config", dir, "set", "fast", "vision", "false"); err != nil {
		t.Fatal(err)
	}
	toml, err := os.ReadFile(filepath.Join(dir, "models.toml"))
	if err != nil || strings.Contains(string(toml), "vision") {
		t.Errorf("false should remove vision from models.toml:\n%s", toml)
	}
}

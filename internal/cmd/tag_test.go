package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mikiobraun/mf/registry"
)

func tagsOf(t *testing.T, dir, name string) []string {
	t.Helper()
	s, err := registry.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := s.Model(name)
	if !ok {
		t.Fatalf("no model %s", name)
	}
	return m.Tags
}

func TestTagAddAndRemove(t *testing.T) {
	dir := scriptingStore(t) // fast has tag cheap

	out, err := runIn(t, "--config", dir, "tag", "add", "fast", "coding", "cheap", "hosted")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"cheap", "coding", "hosted"}; !reflect.DeepEqual(tagsOf(t, dir, "fast"), want) {
		t.Errorf("after add: tags = %v, want %v", tagsOf(t, dir, "fast"), want)
	}
	if out != "fast: cheap, coding, hosted\n" {
		t.Errorf("add printed %q", out)
	}

	if _, err := runIn(t, "--config", dir, "tag", "remove", "fast", "cheap", "nope"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"coding", "hosted"}; !reflect.DeepEqual(tagsOf(t, dir, "fast"), want) {
		t.Errorf("after remove: tags = %v, want %v", tagsOf(t, dir, "fast"), want)
	}

	if _, err := runIn(t, "--config", dir, "tag", "remove", "fast", "coding", "hosted"); err != nil {
		t.Fatal(err)
	}
	if got := tagsOf(t, dir, "fast"); got != nil {
		t.Errorf("after removing all: tags = %#v, want nil", got)
	}
}

func TestTagRejectsBadTagsAndUnknownModels(t *testing.T) {
	dir := scriptingStore(t)
	cases := map[string][]string{
		"@ prefix":      {"tag", "add", "fast", "@coding"},
		"comma":         {"tag", "add", "fast", "a,b"},
		"empty":         {"tag", "add", "fast", ""},
		"unknown model": {"tag", "add", "nope", "coding"},
		"no tag":        {"tag", "remove", "fast"},
	}
	for label, args := range cases {
		if _, err := runIn(t, append([]string{"--config", dir}, args...)...); err == nil {
			t.Errorf("%s: want an error", label)
		}
	}
	if got := tagsOf(t, dir, "fast"); !reflect.DeepEqual(got, []string{"cheap"}) {
		t.Errorf("a rejected command changed the tags: %v", got)
	}
	if _, err := runIn(t, "--config", dir, "tag", "add", "fast", "@coding"); err == nil || !strings.Contains(err.Error(), "without the @") {
		t.Errorf("@ prefix: error %v should explain", err)
	}
}

func TestTagSwapMovesTheTag(t *testing.T) {
	dir := scriptingStore(t) // fast has tag cheap, llama has none

	out, err := runIn(t, "--config", dir, "tag", "swap", "llama", "cheap")
	if err != nil {
		t.Fatal(err)
	}
	if got := tagsOf(t, dir, "fast"); got != nil {
		t.Errorf("fast still has tags %v", got)
	}
	if got := tagsOf(t, dir, "llama"); !reflect.DeepEqual(got, []string{"cheap"}) {
		t.Errorf("llama tags = %v, want [cheap]", got)
	}
	if out != "fast: \nllama: cheap\n" {
		t.Errorf("swap printed %q", out)
	}

	// Swapping to the entry that already has it changes nothing.
	if _, err := runIn(t, "--config", dir, "tag", "swap", "llama", "cheap"); err != nil {
		t.Fatal(err)
	}
	if got := tagsOf(t, dir, "llama"); !reflect.DeepEqual(got, []string{"cheap"}) {
		t.Errorf("llama tags = %v after a no-op swap", got)
	}
}

func TestTagSwapGivesANewTag(t *testing.T) {
	dir := scriptingStore(t)
	if _, err := runIn(t, "--config", dir, "tag", "swap", "llama", "fresh"); err != nil {
		t.Fatal(err)
	}
	if got := tagsOf(t, dir, "llama"); !reflect.DeepEqual(got, []string{"fresh"}) {
		t.Errorf("llama tags = %v, want [fresh]", got)
	}
}

func TestTagSwapRefusesATagOnSeveralEntries(t *testing.T) {
	dir := scriptingStore(t)
	if _, err := runIn(t, "--config", dir, "tag", "add", "llama", "cheap"); err != nil {
		t.Fatal(err)
	}
	_, err := runIn(t, "--config", dir, "tag", "swap", "fast", "cheap")
	if err == nil || !strings.Contains(err.Error(), "fast, llama") {
		t.Errorf("err = %v, want a refusal naming both entries", err)
	}
	if got := tagsOf(t, dir, "llama"); !reflect.DeepEqual(got, []string{"cheap"}) {
		t.Errorf("the refused swap changed llama: %v", got)
	}
}

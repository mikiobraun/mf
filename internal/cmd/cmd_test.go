package cmd

import (
	"reflect"
	"testing"

	"github.com/mikiobraun/mf/internal/store"
)

func TestSplitTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"coding", []string{"coding"}},
		{"coding,cheap", []string{"coding", "cheap"}},
		{" coding , cheap ", []string{"coding", "cheap"}},
		{"coding,,cheap,", []string{"coding", "cheap"}},
	}
	for _, c := range cases {
		if got := splitTags(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitTags(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMergeTags(t *testing.T) {
	got, changed := mergeTags([]string{"a", "b"}, []string{"b", "c"})
	if !changed {
		t.Error("changed = false, want true")
	}
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("got %v, want [a b c]", got)
	}
	if _, changed := mergeTags([]string{"a"}, []string{"a"}); changed {
		t.Error("re-adding an existing tag should not report a change")
	}
	if _, changed := mergeTags([]string{"a"}, nil); changed {
		t.Error("adding no tags should not report a change")
	}
}

func TestMatchesAll(t *testing.T) {
	m := store.Model{Name: "x", Tags: []string{"coding", "cheap"}}
	if !matchesAll(m, nil) {
		t.Error("no filter should match everything")
	}
	if !matchesAll(m, []string{"coding"}) {
		t.Error("single matching tag should match")
	}
	if !matchesAll(m, []string{"coding", "cheap"}) {
		t.Error("all tags present should match")
	}
	if matchesAll(m, []string{"coding", "local"}) {
		t.Error("--tags requires every tag to be present")
	}
}

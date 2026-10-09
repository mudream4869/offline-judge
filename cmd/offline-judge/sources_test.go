//go:build js && wasm

package main

import (
	"slices"
	"testing"
)

func TestProblemKey(t *testing.T) {
	kattis := "https://github.com/Kattis/problemtools/tree/master/examples"
	for _, tt := range []struct{ url, id, want string }{
		{defaultSource, "0001-a-plus-b", "0001-a-plus-b"}, // keeps old submissions
		{kattis, "hello", "Kattis/problemtools/examples:hello"},
		{"https://github.com/o/two-sum", "two-sum", "o/two-sum:two-sum"},
	} {
		if got := problemKey(tt.url, tt.id); got != tt.want {
			t.Errorf("problemKey(%s, %s) = %q, want %q", tt.url, tt.id, got, tt.want)
		}
	}
	if got := sourceLabel("not a url"); got != "not a url" {
		t.Errorf("sourceLabel of a bad URL = %q", got)
	}
}

// withText sets memo's cached text for the test, as if loaded from storage.
func withText(t *testing.T, kv map[string]string) {
	t.Helper()
	memo.mu.Lock()
	saved := memo.text
	memo.text = kv
	memo.mu.Unlock()
	t.Cleanup(func() {
		memo.mu.Lock()
		memo.text = saved
		memo.mu.Unlock()
	})
}

func TestSourceURLs(t *testing.T) {
	other := "https://github.com/o/r"
	for _, tt := range []struct {
		name string
		kv   map[string]string
		want []string
	}{
		{"default", map[string]string{"sources": "", "source": ""}, []string{defaultSource}},
		{"old single source", map[string]string{"sources": "", "source": other}, []string{other}},
		{"list wins", map[string]string{"sources": `["` + other + `"]`, "source": defaultSource}, []string{other}},
		{"empty list stays empty", map[string]string{"sources": `[]`, "source": ""}, []string{}},
	} {
		withText(t, tt.kv)
		if got := sourceURLs(); !slices.Equal(got, tt.want) {
			t.Errorf("%s: sourceURLs() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

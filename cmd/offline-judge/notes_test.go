//go:build js && wasm

package main

import (
	"slices"
	"testing"
)

func TestStarred(t *testing.T) {
	withText(t, map[string]string{"starred": "[]"})
	setStarred("a", true)
	setStarred("b", true)
	setStarred("a", true) // twice is once
	if got := starred(); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("starred = %v", got)
	}
	setStarred("a", false)
	setStarred("x", false) // not starred: no-op
	if got := starred(); !slices.Equal(got, []string{"b"}) {
		t.Errorf("after unstar = %v", got)
	}
}

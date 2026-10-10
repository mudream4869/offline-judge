//go:build js && wasm

package main

import "testing"

func TestUserTemplate(t *testing.T) {
	lg := langs[0]
	withText(t, map[string]string{
		templateKey(lg): "",
		"code_py_a":     lg.code, // untouched editor
		"code_py_b":     "mine",
	})
	if userTemplate(lg) != lg.code {
		t.Fatal("no template set should be the built-in one")
	}
	setUserTemplate(lg, "# mine\n")
	if userTemplate(lg) != "# mine\n" {
		t.Errorf("template = %q", userTemplate(lg))
	}
	memo.mu.Lock()
	_, a := memo.text["code_py_a"]
	b := memo.text["code_py_b"]
	memo.mu.Unlock()
	if a || b != "mine" {
		t.Errorf("untouched editor kept: %v; edited one = %q", a, b)
	}
	setUserTemplate(lg, lg.code)
	if memo.getText(templateKey(lg), "x") != "" {
		t.Error("the built-in template should be saved as empty")
	}
}

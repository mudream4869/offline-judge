package problems

import (
	"bytes"
	"flag"
	"os"
	"testing"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// TestAll checks the problems in this directory.
func TestAll(t *testing.T) {
	ps, err := Load(os.DirFS("."))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) == 0 {
		t.Fatal("no problems")
	}

	for _, p := range ps {
		if p.Title == "" || p.Statement == "" {
			t.Errorf("%s: missing title or statement", p.ID)
		}
		if len(p.Samples()) == 0 {
			t.Errorf("%s: no samples", p.ID)
		}
		if p.Cases[0].Name != p.Samples()[0].Name {
			t.Errorf("%s: samples should come first", p.ID)
		}
		for _, c := range p.Cases {
			if c.Output == "" {
				t.Errorf("%s/%s: empty output", p.ID, c.Name)
			}
			// Sanity: an output always matches itself.
			if !judge.Equal(c.Output, c.Output) {
				t.Errorf("%s/%s: output does not match itself", p.ID, c.Name)
			}
		}
	}
}

var update = flag.Bool("update", false, "rewrite problems.json")

// TestList checks problems.json matches every problem.json.
// Run `go test ./problems -update` after changing a problem.json.
func TestList(t *testing.T) {
	want, err := MakeList(os.DirFS("."))
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile("problems.json", want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile("problems.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("problems.json is outdated; run go test ./problems -update")
	}
	es, err := ParseList(got)
	if err != nil || len(es) == 0 || es[0].Title == "" {
		t.Errorf("ParseList = %+v, %v", es, err)
	}
}

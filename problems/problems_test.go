package problems

import (
	"testing"

	"github.com/mudream4869/offline-judge/internal/judge"
)

func TestAll(t *testing.T) {
	ps, err := All()
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

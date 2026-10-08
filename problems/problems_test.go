package problems

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		if _, err := time.Parse(time.DateTime, p.Version); err != nil {
			t.Errorf("%s: version should be like 2026-10-08 15:04:05: %q", p.ID, p.Version)
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

// checkerHarness prints the cases (JSON on stdin) whose answer the
// checker (argv[2]) doesn't accept.
const checkerHarness = `
import { readFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
const check = (await import(pathToFileURL(process.argv[2]))).default
for (const c of JSON.parse(readFileSync(0, 'utf8'))) {
  const r = await check(c.Input, c.Output, c.Output)
  if (r !== true) console.log(c.Name + ': ' + r)
}
`

// TestCheckers checks every checker accepts the expected outputs.
func TestCheckers(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	ps, err := Load(os.DirFS("."))
	if err != nil {
		t.Fatal(err)
	}
	harness := filepath.Join(t.TempDir(), "harness.mjs")
	if err := os.WriteFile(harness, []byte(checkerHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.Checker == "" {
			continue
		}
		in, err := json.Marshal(p.Cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(node, harness, filepath.Join(p.ID, CheckerFile))
		cmd.Stdin = bytes.NewReader(in)
		out, err := cmd.CombinedOutput()
		if err != nil || len(out) > 0 {
			t.Errorf("%s: checker rejects expected outputs: %v\n%s", p.ID, err,
				strings.TrimSpace(string(out)))
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

func TestSplitHint(t *testing.T) {
	stmt, hint := splitHint("# T\n\nbody\n\n## 提示\n\nuse dict\n\n## 備註\n\nnote\n")
	if stmt != "# T\n\nbody\n\n## 備註\n\nnote\n" || hint != "use dict" {
		t.Errorf("got %q, %q", stmt, hint)
	}
	if stmt, hint := splitHint("# T\n"); stmt != "# T\n" || hint != "" {
		t.Errorf("no hint: got %q, %q", stmt, hint)
	}
}

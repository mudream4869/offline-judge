package problems

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// langIDs are the language ids of cmd/offline-judge.
var langIDs = map[string]bool{"py": true, "cpp": true, "js": true, "go": true}

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
		if len(p.Tags)+len(p.SolutionTags) == 0 {
			t.Errorf("%s: no tags", p.ID)
		}
		for _, tag := range p.SolutionTags {
			if slices.Contains(p.Tags, tag) {
				t.Errorf("%s: %q in both tags and solution_tags", p.ID, tag)
			}
		}
		for lang := range p.TimeLimits {
			if !langIDs[lang] {
				t.Errorf("%s: unknown language in time_limits_ms: %q", p.ID, lang)
			}
		}
		if len(p.Samples()) == 0 {
			t.Errorf("%s: no samples", p.ID)
		}
		if p.Cases[0].Name != p.Samples()[0].Name {
			t.Errorf("%s: samples should come first", p.ID)
		}
		for _, c := range p.Cases {
			// An interactive problem's .out is only a sample transcript.
			if c.Output == "" && (p.Interactor == "" || strings.HasPrefix(c.Name, "sample")) {
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

// interactHarness runs argv[3] (a Python solution) against interactor
// argv[2] on each case (JSON on stdin) and prints the ones it doesn't pass.
// The interactor reads whenever the solution has written whole lines, so
// the solution must flush after each line.
const interactHarness = `
import { readFileSync } from 'node:fs'
import { spawn } from 'node:child_process'
import { pathToFileURL } from 'node:url'
const make = (await import(pathToFileURL(process.argv[2]))).default
for (const c of JSON.parse(readFileSync(0, 'utf8'))) {
  const ia = make(c.Input)
  const p = spawn('python3', [process.argv[3]])
  const send = (r) => (r == null || r === '' ? p.stdin.end() : p.stdin.write(r))
  p.stdin.on('error', () => {})
  send(ia.read(''))
  // A pipe can split a line; pass whole lines, as the program would
  // have written them by the time it reads.
  let pending = ''
  p.stdout.on('data', (d) => {
    pending += d
    if (pending.endsWith('\n') && !p.stdin.writableEnded) {
      send(ia.read(pending))
      pending = ''
    }
  })
  let err = ''
  p.stderr.on('data', (d) => { err += d })
  const code = await new Promise((ok) => p.on('close', ok))
  const r = ia.finish(pending)
  if (r !== true || code !== 0) console.log(c.Name + ': ' + r + ' (exit ' + code + ') ' + err)
}
`

// TestInteractors runs each interactive problem's solution.py against its
// interactor.
func TestInteractors(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}
	ps, err := Load(os.DirFS("."))
	if err != nil {
		t.Fatal(err)
	}
	harness := filepath.Join(t.TempDir(), "harness.mjs")
	if err := os.WriteFile(harness, []byte(interactHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.Interactor == "" {
			continue
		}
		sol := filepath.Join(p.ID, "solution.py")
		if _, err := os.Stat(sol); err != nil {
			t.Errorf("%s: interactive problem without solution.py", p.ID)
			continue
		}
		in, err := json.Marshal(p.Cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(node, harness, filepath.Join(p.ID, InteractorFile), sol)
		cmd.Stdin = bytes.NewReader(in)
		out, err := cmd.CombinedOutput()
		if err != nil || len(out) > 0 {
			t.Errorf("%s: solution fails: %v\n%s", p.ID, err, strings.TrimSpace(string(out)))
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

func TestTimeLimitFor(t *testing.T) {
	m, err := ParseMeta([]byte(`{"title": "x", "time_limit_ms": 2000, "time_limits_ms": {"cpp": 500, "js": 0}}`))
	if err != nil {
		t.Fatal(err)
	}
	p := &Problem{TimeLimit: m.TimeLimit, TimeLimits: m.TimeLimits}
	for lang, want := range map[string]time.Duration{
		"cpp": 500 * time.Millisecond,
		"py":  2 * time.Second,
		"js":  2 * time.Second, // non-positive is ignored
	} {
		if got := p.TimeLimitFor(lang); got != want {
			t.Errorf("TimeLimitFor(%q) = %v, want %v", lang, got, want)
		}
	}
}

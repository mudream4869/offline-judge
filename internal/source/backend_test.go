package source

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
)

// memBackend serves files at one version, counting fetches.
type memBackend struct {
	version string
	files   map[string][]byte
	links   map[string]bool // paths that are symbolic links

	mu      sync.Mutex // Fetch runs in parallel
	fetched []string
}

func (m *memBackend) Latest(context.Context) (string, error) { return m.version, nil }

func (m *memBackend) List(context.Context, string) ([]File, error) {
	var out []File
	for p, bs := range m.files {
		out = append(out, File{Path: p, SHA: BlobSHA(bs), Link: m.links[p]})
	}
	return out, nil
}

func (m *memBackend) Fetch(_ context.Context, _ string, f File) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetched = append(m.fetched, f.Path)
	return m.files[f.Path], nil
}

// TestBackend runs a Set on a Backend other than GitHub.
func TestBackend(t *testing.T) {
	ctx := context.Background()
	b := &memBackend{version: "v1", files: map[string][]byte{}}
	fsys := os.DirFS("../../problems")
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(p == listFile || strings.HasPrefix(p, "0001-a-plus-b/")) {
			return err
		}
		b.files[p], err = fs.ReadFile(fsys, p)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	s := NewFromBackend("mem:test", b, NewMemStore())
	if _, err := s.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if s.Commit() != "v1" || len(b.fetched) != 1 || b.fetched[0] != listFile {
		t.Fatalf("version %q, fetched %v; want v1 and only %s", s.Commit(), b.fetched, listFile)
	}
	p, err := s.Problem(ctx, "0001-a-plus-b")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "A + B" || len(p.Cases) == 0 {
		t.Errorf("problem = %+v", p)
	}
	// Only what the problem needs, e.g. not problem.json or _solutions.
	for _, f := range b.fetched {
		if strings.HasSuffix(f, "problem.json") || strings.Contains(f, "_solutions") {
			t.Errorf("fetched %s", f)
		}
	}
}

// TestKattisSource reads a directory of Kattis packages.
func TestKattisSource(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"README.md":                              "contest",
		"hello/problem.yaml":                     "name: Hello World!\n",
		"hello/problem_statement/problem.en.tex": "\\problemname{Hello World!}\nSay hello.\n",
		"hello/data/secret/hello.in":             "",
		"hello/data/secret/hello.ans":            "Hello World!\n",
		"hello/data/secret/g2/again.in":          "../hello.in",
		"hello/data/secret/g2/again.ans":         "../hello.ans",
		"hello/submissions/accepted/a.py":        "print('Hello World!')",
		"hello/input_validators/v.py":            "",
		"guess/problem.yaml":                     "type: interactive\nname: Guess\n",
		"guess/statement/problem.en.md":          "Guess.\n",
		"guess/data/secret/1.in":                 "5\n",
		"guess/output_validator/v.cc":            "",
	}
	b := &memBackend{version: "v1", files: map[string][]byte{}, links: map[string]bool{
		"hello/data/secret/g2/again.in": true, "hello/data/secret/g2/again.ans": true,
	}}
	for p, s := range files {
		b.files[p] = []byte(s)
	}
	s := NewFromBackend("mem:kattis", b, NewMemStore())
	if _, err := s.Open(ctx); err != nil {
		t.Fatal(err)
	}
	es := s.Entries()
	if len(es) != 2 || es[0].ID != "guess" || es[1].ID != "hello" || es[1].Title != "Hello World!" ||
		es[0].Unsupported == "" || es[1].Unsupported != "" || len(es[1].Version) != 12 {
		t.Fatalf("entries = %+v", es)
	}
	if len(b.fetched) != 2 {
		t.Errorf("list fetched %v, want only the two problem.yaml", b.fetched)
	}

	p, err := s.Problem(ctx, "hello")
	if err != nil {
		t.Fatal(err)
	}
	// The linked test has the target's content.
	if len(p.Cases) != 2 || p.Cases[0].Name != "secret/g2/again" || p.Cases[0].Output != "Hello World!\n" ||
		p.Cases[1].Output != "Hello World!\n" || p.Version != es[1].Version ||
		!strings.Contains(p.Statement, "Say hello.") {
		t.Errorf("problem = %+v", p)
	}
	for _, f := range b.fetched {
		if strings.Contains(f, "submissions") || strings.Contains(f, "validators") {
			t.Errorf("fetched %s", f)
		}
	}
	if p, err := s.Problem(ctx, "guess"); err != nil || p.Unsupported == "" {
		t.Errorf("interactive problem = %+v, %v", p, err)
	}

	// Changing a test changes the version; other problems keep theirs.
	b.files["hello/data/secret/hello.ans"] = []byte("Hello, World!\n")
	b.version = "v2"
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	es2 := s.Entries()
	if es2[1].Version == es[1].Version || es2[0].Version != es[0].Version {
		t.Errorf("versions %s→%s, %s→%s", es[1].Version, es2[1].Version, es[0].Version, es2[0].Version)
	}
}

// TestKattisRootSource reads a source that is one Kattis package.
func TestKattisRootSource(t *testing.T) {
	ctx := context.Background()
	b := &memBackend{version: "v1", files: map[string][]byte{
		"README.md":               []byte("one problem"),
		"problem.yaml":            []byte("name: Two Sum\n"),
		"statement/problem.en.md": []byte("Add.\n"),
		"data/secret/1.in":        []byte("1 2\n"),
		"data/secret/1.ans":       []byte("3\n"),
		"submissions/ac/a.py":     []byte(""),
	}}
	s := NewFromBackend("https://example.com/someone/two-sum/", b, NewMemStore())
	if _, err := s.Open(ctx); err != nil {
		t.Fatal(err)
	}
	es := s.Entries()
	if len(es) != 1 || es[0].ID != "two-sum" || es[0].Title != "Two Sum" {
		t.Fatalf("entries = %+v", es)
	}
	p, err := s.Problem(ctx, "two-sum")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Cases) != 1 || p.Cases[0].Output != "3\n" || p.Statement != "# Two Sum\n\nAdd.\n" {
		t.Errorf("problem = %+v", p)
	}
}

// A GitHub source that is one problem is named after its folder, or its repo.
func TestRootID(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/o/two-sum":                           "two-sum",
		"https://github.com/o/two-sum.git":                       "two-sum",
		"https://github.com/o/two-sum/tree/main":                 "two-sum",
		"https://github.com/o/contest/tree/main/problems/hello":  "hello",
		"https://github.com/o/contest/tree/main/problems/hello/": "hello",
	} {
		s, err := New(url, NewClient(), NewMemStore())
		if err != nil {
			t.Fatal(err)
		}
		if s.rootID != want {
			t.Errorf("rootID(%s) = %q, want %q", url, s.rootID, want)
		}
	}
}

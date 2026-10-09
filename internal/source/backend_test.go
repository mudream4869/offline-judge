package source

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// memBackend serves files at one version, counting fetches.
type memBackend struct {
	version string
	files   map[string][]byte
	fetched []string
}

func (m *memBackend) Latest(context.Context) (string, error) { return m.version, nil }

func (m *memBackend) List(context.Context, string) ([]File, error) {
	var out []File
	for p, bs := range m.files {
		out = append(out, File{Path: p, SHA: BlobSHA(bs)})
	}
	return out, nil
}

func (m *memBackend) Fetch(_ context.Context, _ string, f File) ([]byte, error) {
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

package source

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Repo
	}{
		{"https://github.com/a/b", Repo{"a", "b", "HEAD", ""}},
		{"https://github.com/a/b.git/", Repo{"a", "b", "HEAD", ""}},
		{"https://github.com/a/b/tree/main", Repo{"a", "b", "main", ""}},
		{" https://github.com/a/b/tree/v1/x/y ", Repo{"a", "b", "v1", "x/y"}},
	} {
		got, err := Parse(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{
		"https://gitlab.com/a/b",
		"https://github.com/a",
		"https://github.com/a/b/blob/main/x",
		"https://github.com/a/b/tree",
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

// fakeGitHub serves dir as github.com/o/r at commit "c…c", under "problems".
type fakeGitHub struct {
	files map[string][]byte // repo path → data
	mu    sync.Mutex
	raw   int // raw downloads
	api   int
}

const commit = "cccccccccccccccccccccccccccccccccccccccc"

func newFake(t *testing.T) (*fakeGitHub, *Client) {
	f := &fakeGitHub{files: map[string][]byte{"README.md": []byte("hi")}}
	fsys := os.DirFS("../../problems")
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		bs, err := fs.ReadFile(fsys, p)
		f.files["problems/"+p] = bs
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repos/o/r/commits/main", func(w http.ResponseWriter, r *http.Request) {
		f.count(&f.api)
		w.Write([]byte(commit))
	})
	mux.HandleFunc("GET /api/repos/o/r/git/trees/"+commit, func(w http.ResponseWriter, r *http.Request) {
		f.count(&f.api)
		tree := []map[string]string{{"path": "problems", "type": "tree", "sha": "x"}}
		for p, bs := range f.files {
			tree = append(tree, map[string]string{"path": p, "type": "blob", "sha": BlobSHA(bs)})
		}
		json.NewEncoder(w).Encode(map[string]any{"tree": tree})
	})
	mux.HandleFunc("GET /raw/o/r/"+commit+"/", func(w http.ResponseWriter, r *http.Request) {
		f.count(&f.raw)
		bs, ok := f.files[strings.TrimPrefix(r.URL.Path, "/raw/o/r/"+commit+"/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(bs)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, &Client{HTTP: srv.Client(), API: srv.URL + "/api", Raw: srv.URL + "/raw"}
}

func (f *fakeGitHub) count(n *int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	*n++
}

const srcURL = "https://github.com/o/r/tree/main/problems"

func TestSet(t *testing.T) {
	ctx := context.Background()
	gh, c := newFake(t)
	st := NewMemStore()

	s, err := New(srcURL, c, st)
	if err != nil {
		t.Fatal(err)
	}
	if cached, err := s.Open(ctx); err != nil || cached {
		t.Fatalf("Open = %v, %v", cached, err)
	}
	es := s.Entries()
	if len(es) != 3 || es[0].ID != "0001-a-plus-b" || es[0].Title != "A + B" || es[0].Cached {
		t.Fatalf("entries = %+v", es)
	}
	// Only problem.json so far.
	if gh.raw != 3 {
		t.Errorf("raw downloads = %d, want 3", gh.raw)
	}

	p, err := s.Problem(ctx, "0001-a-plus-b")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "A + B" || len(p.Samples()) == 0 || !s.Entries()[0].Cached {
		t.Errorf("problem = %+v", p)
	}

	if err := s.DownloadAll(ctx, nil); err != nil {
		t.Fatal(err)
	}
	for _, e := range s.Entries() {
		if !e.Cached {
			t.Errorf("%s not cached", e.ID)
		}
	}

	// A new Set on the same store works offline.
	raw, api := gh.raw, gh.api
	s2, _ := New(srcURL, &Client{HTTP: c.HTTP, API: "http://0.0.0.0:1", Raw: "http://0.0.0.0:1"}, st)
	if cached, err := s2.Open(ctx); err != nil || !cached {
		t.Fatalf("Open = %v, %v", cached, err)
	}
	if _, err := s2.Problem(ctx, "0003-fibonacci"); err != nil {
		t.Fatal(err)
	}
	if s2.Refresh(ctx) == nil {
		t.Error("Refresh should fail offline")
	}

	// Refresh at the same commit only asks for the commit.
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if gh.raw != raw || gh.api != api+1 {
		t.Errorf("refresh: raw %d→%d, api %d→%d", raw, gh.raw, api, gh.api)
	}
}

func TestRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), API: srv.URL}
	_, err := c.Commit(context.Background(), Repo{"o", "r", "main", ""})
	if err == nil || !strings.Contains(err.Error(), "上限") {
		t.Errorf("err = %v", err)
	}
}

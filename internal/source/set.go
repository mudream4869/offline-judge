package source

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"testing/fstest"
	"time"

	"github.com/mudream4869/offline-judge/problems"
)

// Index is the file list of a source at one commit.
type Index struct {
	Commit  string
	Files   []File
	Checked time.Time // last time Commit was confirmed latest
}

// Store keeps indexes by source URL and file contents by blob sha.
type Store interface {
	Index(url string) (*Index, bool)
	SetIndex(url string, ix *Index)
	Blob(sha string) ([]byte, bool)
	SetBlob(sha string, data []byte)
	BlobSHAs() []string
}

// Entry is a problem in the list.
type Entry struct {
	ID           string
	Title        string
	TimeLimit    time.Duration
	TimeLimits   map[string]time.Duration // per-language overrides
	Version      string
	Tags         []string
	SolutionTags []string // hint at the solution; hidden by default
	Cached       bool     // statement and tests are stored, so it works offline
}

// listFile lists the problems of a source, so the list is one download.
const listFile = "problems.json"

// Set is the problems of one source. problems.json comes with the list;
// statement, checker, interactor and tests are downloaded when a problem is opened.
type Set struct {
	URL    string
	repo   Repo
	client *Client
	store  Store

	mu      sync.Mutex
	ix      *Index
	entries []Entry
	metas   map[string]problems.Meta     // by id
	have    map[string]bool              // blob shas in store
	probs   map[string]*problems.Problem // for ix.Commit
}

// New returns the Set of url.
func New(url string, c *Client, st Store) (*Set, error) {
	r, err := Parse(url)
	if err != nil {
		return nil, err
	}
	return &Set{URL: url, repo: r, client: c, store: st}, nil
}

// Open loads the list from the store, or from GitHub if it isn't stored.
// cached tells which, so the caller can Refresh a stored one.
func (s *Set) Open(ctx context.Context) (cached bool, err error) {
	s.mu.Lock()
	if s.ix != nil {
		s.mu.Unlock()
		return true, nil
	}
	if s.have == nil {
		s.have = map[string]bool{}
		for _, sha := range s.store.BlobSHAs() {
			s.have[sha] = true
		}
	}
	ix, ok := s.store.Index(s.URL)
	// A broken cache is fetched again.
	ok = ok && s.setIndex(ix) == nil
	s.mu.Unlock()
	if ok {
		return true, nil
	}
	return false, s.Refresh(ctx)
}

// Refresh fetches the latest list from GitHub.
func (s *Set) Refresh(ctx context.Context) error {
	commit, err := s.client.Commit(ctx, s.repo)
	if err != nil {
		return err
	}

	s.mu.Lock()
	old := s.ix
	s.mu.Unlock()
	var ix *Index
	if old != nil && old.Commit == commit {
		ix = &Index{Commit: commit, Files: old.Files}
	} else {
		files, err := s.client.Tree(ctx, s.repo, commit)
		if err != nil {
			return err
		}
		ix = &Index{Commit: commit, Files: files}
		// The list needs problems.json; get it before saving ix.
		list, ok := ix.list()
		if !ok {
			return fmt.Errorf("來源缺少 %s", listFile)
		}
		if err := s.fetch(ctx, commit, []File{list}, nil); err != nil {
			return err
		}
	}
	ix.Checked = time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.setIndex(ix); err != nil {
		return err
	}
	s.store.SetIndex(s.URL, ix)
	return nil
}

// setIndex makes ix current. s.mu must be held.
func (s *Set) setIndex(ix *Index) error {
	if s.ix != nil && s.ix.Commit == ix.Commit {
		s.ix = ix
		return nil
	}
	f, ok := ix.list()
	if !ok {
		return fmt.Errorf("來源缺少 %s", listFile)
	}
	bs, ok := s.store.Blob(f.SHA)
	if !ok {
		return fmt.Errorf("快取缺少 %s", listFile)
	}
	list, err := problems.ParseList(bs)
	if err != nil {
		return fmt.Errorf("%s：%w", listFile, err)
	}
	var entries []Entry
	metas := map[string]problems.Meta{}
	for _, e := range list {
		entries = append(entries, Entry{ID: e.ID, Title: e.Title, TimeLimit: e.TimeLimit,
			TimeLimits: e.TimeLimits, Version: e.Version, Tags: e.Tags,
			SolutionTags: e.SolutionTags})
		metas[e.ID] = e.Meta
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	s.ix = ix
	s.entries = entries
	s.metas = metas
	s.probs = map[string]*problems.Problem{}
	return nil
}

// Commit returns the commit of the current list, or "" before Open.
func (s *Set) Commit() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ix == nil {
		return ""
	}
	return s.ix.Commit
}

// Checked returns when the list was last confirmed latest.
func (s *Set) Checked() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ix == nil {
		return time.Time{}
	}
	return s.ix.Checked
}

// Entries lists the problems, sorted by id.
func (s *Set) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.entries))
	for i, e := range s.entries {
		e.Cached = s.hasAll(s.files(e.ID))
		out[i] = e
	}
	return out
}

// Problem returns problem id, downloading what isn't stored.
func (s *Set) Problem(ctx context.Context, id string) (*problems.Problem, error) {
	s.mu.Lock()
	if s.ix == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("題目列表尚未載入")
	}
	if p, ok := s.probs[id]; ok {
		s.mu.Unlock()
		return p, nil
	}
	m, ok := s.metas[id]
	ix, files := s.ix, s.files(id)
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("沒有題目 %s", id)
	}

	if err := s.fetch(ctx, ix.Commit, files, nil); err != nil {
		return nil, err
	}
	fsys := fstest.MapFS{}
	for _, f := range files {
		bs, ok := s.store.Blob(f.SHA)
		if !ok {
			return nil, fmt.Errorf("快取缺少 %s", f.Path)
		}
		fsys[f.Path] = &fstest.MapFile{Data: bs}
	}
	p, err := problems.LoadWithMeta(fsys, id, m)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ix.Commit == ix.Commit {
		s.probs[id] = p
	}
	return p, nil
}

// DownloadAll downloads every problem, so all of them work offline.
func (s *Set) DownloadAll(ctx context.Context, progress func(done, total int)) error {
	s.mu.Lock()
	if s.ix == nil {
		s.mu.Unlock()
		return fmt.Errorf("題目列表尚未載入")
	}
	commit := s.ix.Commit
	var files []File
	for _, e := range s.entries {
		files = append(files, s.files(e.ID)...)
	}
	s.mu.Unlock()
	return s.fetch(ctx, commit, files, progress)
}

// files returns what problem id needs. s.mu must be held.
func (s *Set) files(id string) []File {
	var out []File
	for _, f := range s.ix.Files {
		rest, ok := strings.CutPrefix(f.Path, id+"/")
		if !ok {
			continue
		}
		if rest == "statement.md" || rest == problems.CheckerFile || rest == problems.InteractorFile ||
			(path.Dir(rest) == "tests" && (path.Ext(rest) == ".in" || path.Ext(rest) == ".out")) {
			out = append(out, f)
		}
	}
	return out
}

// hasAll reports whether every file is stored. s.mu must be held.
func (s *Set) hasAll(files []File) bool {
	for _, f := range files {
		if !s.have[f.SHA] {
			return false
		}
	}
	return true
}

// Parallel downloads.
const fetchWorkers = 6

// fetch stores the files that aren't stored yet.
func (s *Set) fetch(ctx context.Context, commit string, files []File,
	progress func(done, total int)) error {

	var todo []File
	s.mu.Lock()
	seen := map[string]bool{}
	for _, f := range files {
		if !s.have[f.SHA] && !seen[f.SHA] {
			seen[f.SHA] = true
			todo = append(todo, f)
		}
	}
	s.mu.Unlock()
	if len(todo) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan File)
	errs := make(chan error, len(todo))
	var wg sync.WaitGroup
	for range min(fetchWorkers, len(todo)) {
		wg.Go(func() {
			for f := range jobs {
				errs <- s.fetchOne(ctx, commit, f)
			}
		})
	}
	go func() {
		defer close(jobs)
		for _, f := range todo {
			select {
			case jobs <- f:
			case <-ctx.Done():
				return
			}
		}
	}()

	var first error
	for done := range len(todo) {
		err := <-errs
		if err != nil && first == nil {
			first = err
			cancel()
		}
		if first == nil && progress != nil {
			progress(done+1, len(todo))
		}
		if first != nil {
			break
		}
	}
	wg.Wait()
	return first
}

func (s *Set) fetchOne(ctx context.Context, commit string, f File) error {
	bs, err := s.client.File(ctx, s.repo, commit, f.Path)
	if err != nil {
		return fmt.Errorf("下載 %s 失敗：%w", f.Path, err)
	}
	if BlobSHA(bs) != f.SHA {
		return fmt.Errorf("下載 %s 失敗：內容與 GitHub 列出的不符", f.Path)
	}
	s.store.SetBlob(f.SHA, bs)
	s.mu.Lock()
	s.have[f.SHA] = true
	s.mu.Unlock()
	return nil
}

// BlobSHA is the git blob sha of data.
func BlobSHA(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// list returns problems.json.
func (ix *Index) list() (File, bool) {
	for _, f := range ix.Files {
		if f.Path == listFile {
			return f, true
		}
	}
	return File{}, false
}

// MemStore is a Store in memory.
type MemStore struct {
	mu    sync.Mutex
	ixs   map[string]*Index
	blobs map[string][]byte
}

func NewMemStore() *MemStore {
	return &MemStore{ixs: map[string]*Index{}, blobs: map[string][]byte{}}
}

func (m *MemStore) Index(url string) (*Index, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ix, ok := m.ixs[url]
	return ix, ok
}

func (m *MemStore) SetIndex(url string, ix *Index) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ixs[url] = ix
}

func (m *MemStore) Blob(sha string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	bs, ok := m.blobs[sha]
	return bs, ok
}

func (m *MemStore) SetBlob(sha string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blobs[sha] = data
}

func (m *MemStore) BlobSHAs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.blobs))
	for sha := range m.blobs {
		out = append(out, sha)
	}
	return out
}

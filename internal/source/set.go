package source

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"testing/fstest"
	"time"

	"github.com/mudream4869/offline-judge/problems"
)

// Index is the file list of a source at one version.
type Index struct {
	Commit  string // the version; a commit for GitHub
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

// Set is the problems of one source. The files the list is built from come
// with the list; the rest are downloaded when a problem is opened.
type Set struct {
	URL     string
	backend Backend
	store   Store

	mu      sync.Mutex
	ix      *Index
	fmt     format // of ix
	entries []Entry
	metas   map[string]problems.Meta     // by id
	have    map[string]bool              // blob shas in store
	probs   map[string]*problems.Problem // for ix.Commit
}

// New returns the Set of a GitHub url, fetched with c.
func New(url string, c *Client, st Store) (*Set, error) {
	r, err := Parse(url)
	if err != nil {
		return nil, err
	}
	return NewFromBackend(url, GitHub{Repo: r, Client: c}, st), nil
}

// NewFromBackend returns the Set of files from b; url keys it in st.
func NewFromBackend(url string, b Backend, st Store) *Set {
	return &Set{URL: url, backend: b, store: st}
}

// Open loads the list from the store, or from the backend if it isn't stored.
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

// Refresh fetches the latest list from the backend.
func (s *Set) Refresh(ctx context.Context) error {
	commit, err := s.backend.Latest(ctx)
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
		files, err := s.backend.List(ctx, commit)
		if err != nil {
			return err
		}
		ix = &Index{Commit: commit, Files: files}
		// Get what the list is built from before saving ix.
		f, err := formatOf(ix)
		if err != nil {
			return err
		}
		need, err := f.listFiles(ix)
		if err != nil {
			return err
		}
		if err := s.fetch(ctx, commit, need, nil); err != nil {
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
	f, err := formatOf(ix)
	if err != nil {
		return err
	}
	list, err := f.list(ix, func(f File) ([]byte, error) {
		bs, ok := s.store.Blob(f.SHA)
		if !ok {
			return nil, fmt.Errorf("快取缺少 %s", f.Path)
		}
		return bs, nil
	})
	if err != nil {
		return err
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
	s.fmt = f
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
	ix, f, files := s.ix, s.fmt, s.files(id)
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
	p, err := f.load(fsys, id, m)
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
	return s.fmt.files(s.ix, id)
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

	if err := context.Cause(ctx); err != nil {
		return err
	}

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
		return context.Cause(ctx)
	}

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	jobs := make(chan File)
	completed := make(chan struct{}, fetchWorkers)
	var wg sync.WaitGroup
	for range min(fetchWorkers, len(todo)) {
		wg.Go(func() {
			for f := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := s.fetchOne(ctx, commit, f); err != nil {
					cancel(err)
				}
				completed <- struct{}{}
			}
		})
	}
	wg.Go(func() {
		defer close(jobs)
		for _, f := range todo {
			if ctx.Err() != nil {
				return
			}
			select {
			case jobs <- f:
			case <-ctx.Done():
				return
			}
		}
	})
	go func() {
		wg.Wait()
		close(completed)
	}()

	// Drain until both dispatch and downloads finish, including on cancel.
	// A canceled batch may have dispatched fewer jobs than len(todo).
	done := 0
	for range completed {
		done++
		if progress != nil && ctx.Err() == nil {
			progress(done, len(todo))
		}
	}
	return context.Cause(ctx)
}

func (s *Set) fetchOne(ctx context.Context, commit string, f File) error {
	bs, err := s.backend.Fetch(ctx, commit, f)
	if err != nil {
		return fmt.Errorf("下載 %s 失敗：%w", f.Path, err)
	}
	if BlobSHA(bs) != f.SHA {
		return fmt.Errorf("下載 %s 失敗：內容與來源列出的不符", f.Path)
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

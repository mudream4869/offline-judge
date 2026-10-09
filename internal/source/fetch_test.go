package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fetchSet(t *testing.T, n int, transport roundTripFunc) (*Set, []File) {
	t.Helper()
	s, err := New(srcURL, &Client{HTTP: &http.Client{Transport: transport}, Raw: "https://raw.invalid"}, NewMemStore())
	if err != nil {
		t.Fatal(err)
	}
	s.have = map[string]bool{}
	var files []File
	for i := range n {
		name := fmt.Sprintf("file-%02d", i)
		files = append(files, File{Path: name, SHA: BlobSHA([]byte(name))})
	}
	return s, files
}

func fileResponse(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(path.Base(r.URL.Path))), Header: http.Header{}}, nil
}

func fetchResult(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("fetch did not finish")
		return nil
	}
}

func waitFetchSignal(t *testing.T, ch <-chan struct{}, n int) {
	t.Helper()
	for range n {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("download worker did not reach the expected state")
		}
	}
}

func TestFetchAlreadyCanceled(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("deadline=%v/cached=%v", deadline, cached), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				want := context.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
					want = context.DeadlineExceeded
				}
				cancel()
				var calls atomic.Int32
				s, files := fetchSet(t, fetchWorkers*2, func(r *http.Request) (*http.Response, error) {
					calls.Add(1)
					return nil, r.Context().Err()
				})
				if cached {
					for _, f := range files {
						s.have[f.SHA] = true
						s.store.SetBlob(f.SHA, []byte(f.Path))
					}
				}
				result := make(chan error, 1)
				go func() { result <- s.fetch(ctx, commit, files, func(int, int) { t.Error("unexpected progress") }) }()
				if err := fetchResult(t, result); !errors.Is(err, want) {
					t.Errorf("fetch = %v, want %v", err, want)
				}
				if calls.Load() != 0 {
					t.Errorf("started %d requests after cancellation", calls.Load())
				}
			})
		}
	}
}

func TestFetchCancellationWaitsForWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, fetchWorkers)
	canceled := make(chan struct{}, fetchWorkers)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var calls, active atomic.Int32
	s, files := fetchSet(t, fetchWorkers*3, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		active.Add(1)
		defer active.Add(-1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		select {
		case canceled <- struct{}{}:
		default:
		}
		<-release
		return nil, r.Context().Err()
	})
	result := make(chan error, 1)
	go func() { result <- s.fetch(ctx, commit, files, nil) }()
	waitFetchSignal(t, started, fetchWorkers)
	cancel()
	waitFetchSignal(t, canceled, fetchWorkers)
	select {
	case err := <-result:
		t.Fatalf("fetch returned before workers finished: %v", err)
	default:
	}
	unblock()
	if err := fetchResult(t, result); !errors.Is(err, context.Canceled) {
		t.Errorf("fetch = %v, want cancellation", err)
	}
	if calls.Load() != fetchWorkers || active.Load() != 0 {
		t.Errorf("requests = %d, active = %d", calls.Load(), active.Load())
	}
}

func TestFetchFailurePreservesCauseAndWaitsForWorkers(t *testing.T) {
	failure := errors.New("download failed")
	started := make(chan struct{}, fetchWorkers)
	canceled := make(chan struct{}, fetchWorkers)
	fail := make(chan struct{})
	release := make(chan struct{})
	trigger := sync.OnceFunc(func() { close(fail) })
	unblock := sync.OnceFunc(func() { close(release) })
	defer trigger()
	defer unblock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls, active atomic.Int32
	s, files := fetchSet(t, fetchWorkers*3, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		active.Add(1)
		defer active.Add(-1)
		select {
		case started <- struct{}{}:
		default:
		}
		if path.Base(r.URL.Path) == "file-00" {
			<-fail
			return nil, failure
		}
		<-r.Context().Done()
		select {
		case canceled <- struct{}{}:
		default:
		}
		<-release
		return nil, r.Context().Err()
	})
	result := make(chan error, 1)
	go func() { result <- s.fetch(ctx, commit, files, nil) }()
	waitFetchSignal(t, started, fetchWorkers)
	trigger()
	waitFetchSignal(t, canceled, fetchWorkers-1)
	select {
	case err := <-result:
		t.Fatalf("fetch returned before workers finished: %v", err)
	default:
	}
	unblock()
	if err := fetchResult(t, result); !errors.Is(err, failure) || !strings.Contains(err.Error(), "file-00") {
		t.Errorf("fetch = %v, want original file-00 failure", err)
	}
	if calls.Load() != fetchWorkers || active.Load() != 0 {
		t.Errorf("requests = %d, active = %d", calls.Load(), active.Load())
	}
}

func TestFetchProgressCancellationAndRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var retry atomic.Bool
	var mu sync.Mutex
	calls := map[string]int{}
	s, files := fetchSet(t, fetchWorkers*3, func(r *http.Request) (*http.Response, error) {
		name := path.Base(r.URL.Path)
		mu.Lock()
		calls[name]++
		mu.Unlock()
		if retry.Load() || name == "file-00" {
			return fileResponse(r)
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	progress := 0
	result := make(chan error, 1)
	go func() {
		result <- s.fetch(ctx, commit, files, func(done, total int) {
			progress++
			if done != 1 || total != len(files) {
				t.Errorf("progress = %d/%d", done, total)
			}
			cancel()
		})
	}()
	if err := fetchResult(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("fetch = %v, want cancellation", err)
	}
	if progress != 1 || len(s.store.BlobSHAs()) != 1 {
		t.Fatalf("progress calls = %d, cached blobs = %d", progress, len(s.store.BlobSHAs()))
	}
	retry.Store(true)
	progress = 0
	// Duplicate entries should not inflate downloads or the progress total.
	if err := s.fetch(context.Background(), commit, append(files, files...), func(done, total int) {
		progress++
		if done != progress || total != len(files)-1 {
			t.Errorf("retry progress = %d/%d, call %d", done, total, progress)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if progress != len(files)-1 || !s.hasAll(files) {
		t.Errorf("retry progress = %d, all cached = %v", progress, s.hasAll(files))
	}
	if calls["file-00"] != 1 {
		t.Errorf("completed file downloaded %d times", calls["file-00"])
	}
	if err := s.fetch(context.Background(), commit, files, func(int, int) { t.Error("unexpected cached progress") }); err != nil {
		t.Fatal(err)
	}
}

func TestFetchRejectsCorruptBlob(t *testing.T) {
	s, files := fetchSet(t, 1, fileResponse)
	files[0].SHA = BlobSHA([]byte("different content"))
	if err := s.fetch(context.Background(), commit, files, nil); err == nil || !strings.Contains(err.Error(), "內容與 GitHub 列出的不符") {
		t.Errorf("fetch = %v, want blob mismatch", err)
	}
	if s.have[files[0].SHA] || len(s.store.BlobSHAs()) != 0 {
		t.Error("corrupt file was cached")
	}
}

func TestFetchCancelOnFinalProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, files := fetchSet(t, 1, fileResponse)
	if err := s.fetch(ctx, commit, files, func(int, int) { cancel() }); !errors.Is(err, context.Canceled) {
		t.Errorf("fetch = %v, want cancellation", err)
	}
	if !s.hasAll(files) {
		t.Error("completed file was not kept in the cache")
	}
}

func TestFetchParentCancellationCause(t *testing.T) {
	cause := errors.New("source changed")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	s, files := fetchSet(t, 1, fileResponse)
	if err := s.fetch(ctx, commit, files, func(int, int) { cancel(cause) }); !errors.Is(err, cause) {
		t.Errorf("fetch = %v, want parent cancellation cause", err)
	}
}

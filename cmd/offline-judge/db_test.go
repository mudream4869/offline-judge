//go:build js && wasm

package main

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/source"
	"github.com/mudream4869/offline-judge/internal/submissions"
)

// Abort after a request succeeds, exercising rollback after a key is allocated.
func abortWrites(t *testing.T, method string) func() {
	t.Helper()
	proto := js.Global().Get("IDBObjectStore").Get("prototype")
	original := proto.Get(method)
	abort := js.FuncOf(func(_ js.Value, args []js.Value) any {
		args[0].Get("target").Get("transaction").Call("abort")
		return nil
	})
	intercept := js.FuncOf(func(this js.Value, args []js.Value) any {
		req := original.Call("call", this, args[0])
		req.Call("addEventListener", "success", abort, map[string]any{"once": true})
		return req
	})
	proto.Set(method, intercept)
	restore := func() {
		proto.Set(method, original)
		intercept.Release()
		abort.Release()
	}
	return restore
}

func browserSubmission(problem string) *submission {
	return &submission{Problem: problem, Version: "v1", Lang: "js", Code: "console.log(1)", At: time.Now(), Report: judge.Report{Verdict: judge.AC}}
}

func TestSubmissionCommitAndDelete(t *testing.T) {
	sub := browserSubmission(t.Name())
	if err := saveSubmission(sub); err != nil {
		t.Fatal(err)
	}
	if sub.ID <= 0 {
		t.Fatalf("saved ID = %d", sub.ID)
	}
	rows, err := loadSubmissions(sub.Problem, 50)
	if err != nil || len(rows) != 1 || rows[0].Code != sub.Code || rows[0].Report.Verdict != judge.AC {
		t.Fatalf("stored rows = %+v, error = %v", rows, err)
	}
	if err := deleteSubmission(sub.ID); err != nil {
		t.Fatal(err)
	}
	rows, err = loadSubmissions(sub.Problem, 50)
	if err != nil || len(rows) != 0 {
		t.Fatalf("deleted rows = %+v, error = %v", rows, err)
	}
}

func TestSubmissionRollbackUsesMemoryAndRecovers(t *testing.T) {
	s := submissions.New(submissionDB{})
	temp := browserSubmission(t.Name())
	func() {
		restore := abortWrites(t, "add")
		defer restore()
		s.Add(temp)
	}()
	if temp.ID >= 0 {
		t.Fatalf("rolled-back submission ID = %d, want temporary", temp.ID)
	}
	rows, err := loadSubmissions(temp.Problem, 50)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rollback rows = %+v, error = %v", rows, err)
	}
	saved := browserSubmission(t.Name())
	s.Add(saved)
	if saved.ID <= 0 || saved.ID == temp.ID {
		t.Fatalf("IDs = %d/%d", temp.ID, saved.ID)
	}
	if rows := s.ForProblem(temp.Problem); len(rows) != 2 || rows[0].ID != saved.ID || rows[1].ID != temp.ID {
		t.Fatalf("merged rows = %+v", rows)
	}
	if err := s.Delete(temp.ID); err != nil {
		t.Fatal(err)
	}
	rows, err = loadSubmissions(saved.Problem, 50)
	if err != nil || len(rows) != 1 || rows[0].ID != saved.ID {
		t.Fatalf("temporary deletion affected saved record: %+v, %v", rows, err)
	}
	if err := s.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSubmissionDeleteRollbackRemainsVisible(t *testing.T) {
	s := submissions.New(submissionDB{})
	sub := browserSubmission(t.Name())
	s.Add(sub)
	func() {
		restore := abortWrites(t, "delete")
		defer restore()
		if err := s.Delete(sub.ID); err == nil {
			t.Fatal("aborted delete reported success")
		}
	}()
	if rows := s.ForProblem(sub.Problem); len(rows) != 1 || rows[0].ID != sub.ID {
		t.Fatalf("record disappeared: %+v", rows)
	}
	if err := s.Delete(sub.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.ForProblem(sub.Problem)) != 0 {
		t.Fatal("retry did not delete record")
	}
}

func TestWriteRequestSynchronousFailure(t *testing.T) {
	_, err := request(subStore, "readwrite", func(st js.Value) js.Value {
		return st.Call("add", js.Global().Get("Function").New("")) // functions cannot be cloned into IndexedDB
	})
	if err == nil {
		t.Fatal("uncloneable value reported success")
	}
	if err := saveSubmission(browserSubmission(t.Name())); err != nil {
		t.Fatalf("write after synchronous failure: %v", err)
	}
}

func TestWriteHelpersCommit(t *testing.T) {
	saveDraft(t.Name(), "draft")
	if value, ok := loadDraft(t.Name()); !ok || value != "draft" {
		t.Fatalf("draft = %q, %v", value, ok)
	}
}

func TestSubmissionNumbers(t *testing.T) {
	for id, want := range map[int]string{1: "#1", 12: "#12", -1: "暫存 #1", -12: "暫存 #12"} {
		if got := submissionNumber(id); got != want {
			t.Errorf("number(%d) = %q", id, got)
		}
	}
}

func TestSubmissionNoDBFallback(t *testing.T) {
	_ = db()
	previous := dbVal
	dbVal = js.Undefined()
	defer func() { dbVal = previous }()
	s := submissions.New(submissionDB{})
	sub := browserSubmission(t.Name())
	s.Add(sub)
	if sub.ID >= 0 || len(s.All()) != 1 {
		t.Fatal("unavailable database lost submission")
	}
	if err := s.Delete(sub.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.All()) != 0 {
		t.Fatal("temporary deletion failed")
	}
	if err := saveSubmission(sub); !errors.Is(err, errNoDB) {
		t.Fatalf("save = %v", err)
	}
}

func TestWriteRequestErrorAfterPreventingAbort(t *testing.T) {
	sub := browserSubmission(t.Name())
	if err := saveSubmission(sub); err != nil {
		t.Fatal(err)
	}
	prevent := js.FuncOf(func(_ js.Value, args []js.Value) any {
		args[0].Call("preventDefault")
		return nil
	})
	defer prevent.Release()
	_, err := request(subStore, "readwrite", func(st js.Value) js.Value {
		req := st.Call("add", map[string]any{"id": sub.ID})
		req.Call("addEventListener", "error", prevent, map[string]any{"once": true})
		return req
	})
	if err == nil || !strings.Contains(err.Error(), "ConstraintError") {
		t.Fatalf("duplicate key = %v", err)
	}
	if err := deleteSubmission(sub.ID); err != nil {
		t.Fatal(err)
	}
}

func TestProblemCacheWriteHelpersCommit(t *testing.T) {
	st := idbStore{mem: source.NewMemStore()}
	data := []byte("cached content")
	sha := source.BlobSHA(data)
	st.SetBlob(sha, data)
	if got, ok := st.Blob(sha); !ok || !bytes.Equal(got, data) {
		t.Fatalf("blob = %q, %v", got, ok)
	}
	if !slices.Contains(st.BlobSHAs(), sha) {
		t.Fatal("blob key was not persisted")
	}
	st.SetIndex(t.Name(), &source.Index{Commit: "cached commit"})
	if ix, ok := st.Index(t.Name()); !ok || ix.Commit != "cached commit" {
		t.Fatalf("index = %+v, %v", ix, ok)
	}
}

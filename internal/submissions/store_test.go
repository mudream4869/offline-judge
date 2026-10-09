package submissions

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

var errBackend = errors.New("storage unavailable")

type fakeBackend struct {
	rows                         []*Submission
	next                         int
	writeErr, readErr, deleteErr error
	deletes                      []int
	loads, allLoads              int
}

func (b *fakeBackend) Save(sub *Submission) error {
	// An aborted transaction may have allocated a key without committing it.
	sub.ID = b.next + 1
	if b.writeErr != nil {
		return b.writeErr
	}
	b.next++
	copy := *sub
	b.rows = append([]*Submission{&copy}, b.rows...)
	return nil
}
func (b *fakeBackend) Load(problem string, limit int) ([]*Submission, error) {
	b.loads++
	if b.readErr != nil {
		return nil, b.readErr
	}
	var rows []*Submission
	for _, sub := range b.rows {
		if sub.Problem == problem && len(rows) < limit {
			rows = append(rows, sub)
		}
	}
	return rows, nil
}
func (b *fakeBackend) LoadAll() ([]*Submission, error) {
	b.allLoads++
	return b.rows, b.readErr
}
func (b *fakeBackend) Delete(id int) error {
	b.deletes = append(b.deletes, id)
	if b.deleteErr != nil {
		return b.deleteErr
	}
	b.rows = slices.DeleteFunc(b.rows, func(sub *Submission) bool { return sub.ID == id })
	return nil
}
func ids(rows []*Submission) []int {
	out := make([]int, len(rows))
	for i, row := range rows {
		out[i] = row.ID
	}
	return out
}
func wantIDs(t *testing.T, rows []*Submission, want ...int) {
	t.Helper()
	if got := ids(rows); !slices.Equal(got, want) {
		t.Fatalf("IDs = %v, want %v", got, want)
	}
}

func TestSaveFailureRecoveryUsesDistinctIDs(t *testing.T) {
	b := &fakeBackend{}
	s := New(b)
	s.Add(&Submission{Problem: "p", Code: "saved before"})
	b.writeErr = errBackend
	s.Add(&Submission{Problem: "p", Code: "temporary one"})
	s.Add(&Submission{Problem: "q", Code: "temporary two"})
	b.writeErr = nil
	s.Add(&Submission{Problem: "p", Code: "saved after", At: time.Unix(1, 0)})
	wantIDs(t, s.All(), 2, -2, -1, 1)
	wantIDs(t, s.ForProblem("p"), 2, -1, 1)
	wantIDs(t, s.ForProblem("q"), -2)
	// A new page contains only committed records.
	wantIDs(t, New(b).All(), 2, 1)
}

func TestTemporaryDeletionDoesNotReachBackend(t *testing.T) {
	b := &fakeBackend{writeErr: errBackend}
	s := New(b)
	first := &Submission{Problem: "p"}
	s.Add(first)
	if err := s.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	b.writeErr = nil
	s.Add(&Submission{Problem: "p"})
	b.writeErr = errBackend
	s.Add(&Submission{Problem: "p"})
	wantIDs(t, s.All(), -2, 1)
	if err := s.Delete(-2); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, s.All(), 1)
	if len(b.deletes) != 0 {
		t.Fatalf("temporary deletions reached database: %v", b.deletes)
	}
	if len(b.rows) != 1 {
		t.Fatal("saved record was deleted")
	}
}

func TestFailedReadsRetryWithoutLosingSessionRecords(t *testing.T) {
	b := &fakeBackend{}
	old := &Submission{Problem: "p", Code: "old"}
	_ = b.Save(old)
	s := New(b)
	b.readErr = errBackend
	s.Add(&Submission{Problem: "p", Code: "saved during failed reads"})
	b.writeErr = errBackend
	s.Add(&Submission{Problem: "p", Code: "temporary"})
	wantIDs(t, s.ForProblem("p"), -1, 2)
	wantIDs(t, s.All(), -1, 2)
	b.readErr = nil
	wantIDs(t, s.ForProblem("p"), -1, 2, 1)
	wantIDs(t, s.All(), -1, 2, 1)
	if b.loads != 2 || b.allLoads != 2 {
		t.Errorf("reads = %d/%d, want two retries", b.loads, b.allLoads)
	}
}

func TestFailedFullScanShowsCachedProblemHistory(t *testing.T) {
	b := &fakeBackend{}
	_ = b.Save(&Submission{Problem: "p"})
	_ = b.Save(&Submission{Problem: "q"})
	s := New(b)
	wantIDs(t, s.ForProblem("p"), 1)
	b.readErr = errBackend
	wantIDs(t, s.All(), 1)
	b.readErr = nil
	wantIDs(t, s.All(), 2, 1)
}

func TestDeletionFailureKeepsRecordVisible(t *testing.T) {
	b := &fakeBackend{}
	_ = b.Save(&Submission{Problem: "p"})
	s := New(b)
	wantIDs(t, s.ForProblem("p"), 1)
	wantIDs(t, s.All(), 1)
	b.deleteErr = errBackend
	if err := s.Delete(1); !errors.Is(err, errBackend) {
		t.Fatalf("Delete = %v", err)
	}
	wantIDs(t, s.ForProblem("p"), 1)
	wantIDs(t, s.All(), 1)
	b.deleteErr = nil
	if err := s.Delete(1); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, s.ForProblem("p"))
	wantIDs(t, s.All())
	wantIDs(t, New(b).All())
}

func TestDeletionRefillsHistoryAndRetainsCacheOnReadFailure(t *testing.T) {
	b := &fakeBackend{}
	for range HistoryLimit + 1 {
		_ = b.Save(&Submission{Problem: "p"})
	}
	s := New(b)
	if len(s.ForProblem("p")) != HistoryLimit {
		t.Fatal("history window is not full")
	}
	b.readErr = errBackend
	if err := s.Delete(HistoryLimit + 1); err != nil {
		t.Fatal(err)
	}
	if len(s.ForProblem("p")) != HistoryLimit-1 {
		t.Fatal("failed read discarded cached history")
	}
	b.readErr = nil
	history := s.ForProblem("p")
	if len(history) != HistoryLimit || history[len(history)-1].ID != 1 {
		t.Fatalf("history not refilled: %v", ids(history))
	}
}

func TestHistoryLimitPreservesAllTemporaryRecords(t *testing.T) {
	b := &fakeBackend{writeErr: errBackend, readErr: errBackend}
	s := New(b)
	for range HistoryLimit + 5 {
		s.Add(&Submission{Problem: "p"})
	}
	if len(s.ForProblem("p")) != HistoryLimit || len(s.All()) != HistoryLimit+5 {
		t.Fatal("history truncation lost records from full list")
	}
	if err := s.Delete(-55); err != nil {
		t.Fatal(err)
	}
	if len(s.ForProblem("p")) != HistoryLimit {
		t.Fatal("temporary deletion did not refill from memory")
	}
}

func TestSessionOrderingDoesNotDependOnClockOrIDSign(t *testing.T) {
	b := &fakeBackend{}
	s := New(b)
	for i := range 4 {
		if i%2 == 0 {
			b.writeErr = nil
		} else {
			b.writeErr = errBackend
		}
		s.Add(&Submission{Problem: "p", At: time.Unix(int64(4-i), 0)})
	}
	wantIDs(t, s.All(), -2, 2, -1, 1)
}

func TestAddSnapshotsReusedInput(t *testing.T) {
	b := &fakeBackend{writeErr: errBackend}
	s := New(b)
	sub := &Submission{Problem: "p", Code: "first"}
	s.Add(sub)
	sub.Code = "second"
	s.Add(sub)
	rows := s.All()
	wantIDs(t, rows, -2, -1)
	if rows[1].Code != "first" {
		t.Fatal("reused input changed earlier submission")
	}
}

func TestConcurrentFallbackIDsAreUnique(t *testing.T) {
	b := &fakeBackend{writeErr: errBackend}
	s := New(b)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() { s.Add(&Submission{Problem: "p"}) })
	}
	wg.Wait()
	rows := s.All()
	if len(rows) != 100 {
		t.Fatalf("got %d records", len(rows))
	}
	seen := map[int]bool{}
	for _, sub := range rows {
		if sub.ID >= 0 || seen[sub.ID] {
			t.Fatalf("invalid or duplicate temporary ID %d", sub.ID)
		}
		seen[sub.ID] = true
	}
}

// Package submissions keeps submission history when persistence is unavailable.
package submissions

import (
	"slices"
	"sync"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// Submission is one judged submission. Positive IDs are persisted; negative
// IDs identify records kept only in this Store until the page is reloaded.
type Submission struct {
	ID      int
	Problem string
	Version string
	Lang    string
	Code    string
	At      time.Time
	Report  judge.Report
}

// Backend loads newest first. Save assigns a positive ID only after commit.
type Backend interface {
	Load(problem string, limit int) ([]*Submission, error)
	LoadAll() ([]*Submission, error)
	Save(*Submission) error
	Delete(id int) error
}

const HistoryLimit = 50

type cache struct {
	rows   []*Submission
	loaded bool
}

// Store merges records added this session with cached database history.
// Failed reads are retried; failed writes remain available in memory.
type Store struct {
	mu        sync.Mutex
	backend   Backend
	nextID    int
	added     []*Submission // newest first, including successful saves
	byProblem map[string]*cache
	all       cache
}

func New(backend Backend) *Store {
	return &Store{backend: backend, byProblem: map[string]*cache{}}
}

func (s *Store) Add(sub *Submission) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := *sub
	record.ID = 0
	if err := s.backend.Save(&record); err != nil {
		s.nextID--
		record.ID = s.nextID
	}
	sub.ID = record.ID
	s.added = append([]*Submission{&record}, s.added...)
}

func (s *Store) ForProblem(problem string) []*Submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.all.rows
	if !s.all.loaded {
		c := s.byProblem[problem]
		if c == nil {
			c = &cache{}
			s.byProblem[problem] = c
		}
		if !c.loaded {
			if rows, err := s.backend.Load(problem, HistoryLimit); err == nil {
				c.rows, c.loaded = slices.Clone(rows), true
			}
		}
		rows = c.rows
	}
	return s.merge(rows, problem, HistoryLimit)
}

func (s *Store) All() []*Submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.all.loaded {
		if rows, err := s.backend.LoadAll(); err == nil {
			s.all.rows, s.all.loaded = slices.Clone(rows), true
		}
	}
	rows := s.all.rows
	if !s.all.loaded {
		// A failed full scan still shows previously loaded problem histories.
		rows = slices.Clone(rows)
		for _, c := range s.byProblem {
			rows = append(rows, c.rows...)
		}
		slices.SortFunc(rows, func(a, b *Submission) int {
			if a.ID > b.ID {
				return -1
			}
			if a.ID < b.ID {
				return 1
			}
			return 0
		})
	}
	return s.merge(rows, "", 0)
}

// merge preserves this session's insertion order, even if the clock changes
// or a temporary ID is followed by a smaller, persisted ID.
func (s *Store) merge(rows []*Submission, problem string, limit int) []*Submission {
	var out []*Submission
	seen := map[int]bool{}
	for _, group := range [][]*Submission{s.added, rows} {
		for _, sub := range group {
			if seen[sub.ID] || (problem != "" && sub.Problem != problem) {
				continue
			}
			seen[sub.ID] = true
			out = append(out, sub)
			if limit > 0 && len(out) == limit {
				return out
			}
		}
	}
	return out
}

// Delete never sends a temporary ID to the backend. A failed persistent
// deletion leaves the record visible so the user can retry it.
func (s *Store) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id > 0 {
		if err := s.backend.Delete(id); err != nil {
			return err
		}
	}
	remove := func(rows []*Submission) []*Submission {
		return slices.DeleteFunc(rows, func(sub *Submission) bool { return sub.ID == id })
	}
	s.added = remove(s.added)
	s.all.rows = remove(s.all.rows)
	for _, c := range s.byProblem {
		n := len(c.rows)
		c.rows = remove(c.rows)
		if id > 0 && len(c.rows) != n {
			c.loaded = false // refill the history window after a saved deletion
		}
	}
	return nil
}

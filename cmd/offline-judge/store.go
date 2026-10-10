//go:build js && wasm

package main

import (
	"strings"
	"sync"

	"github.com/mudream4869/offline-judge/internal/submissions"
)

// memo caches drafts and submissions, backed by IndexedDB.
// toolgui starts each page with an empty state, so pages read from here.
var memo = &store{text: map[string]string{}, history: submissions.New(submissionDB{})}

type submission = submissions.Submission

type store struct {
	mu      sync.Mutex
	text    map[string]string
	history *submissions.Store
}

func (s *store) getLang() int {
	id := s.getText("lang", langs[0].id)
	for i, l := range langs {
		if l.id == id {
			return i
		}
	}
	return 0
}

func (s *store) setLang(i int) {
	s.setText("lang", langs[i].id)
}

// getText returns the saved text, or def if there is none.
func (s *store) getText(key, def string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.text[key]; ok {
		return v
	}
	v, ok := loadDraft(key)
	if !ok {
		// Cached but not saved: an untouched draft follows a new default.
		v = def
	}
	s.text[key] = v
	return v
}

func (s *store) setText(key, v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.text[key]; ok && old == v {
		return
	}
	s.text[key] = v
	saveDraft(key, v)
}

// forgetDefaults drops cached texts under prefix that are def, so editors
// left as the old default pick up a new one.
func (s *store) forgetDefaults(prefix, def string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.text {
		if strings.HasPrefix(k, prefix) && v == def {
			delete(s.text, k)
		}
	}
}

func (s *store) submissions(problem string) []*submission {
	return s.history.ForProblem(problem)
}

func (s *store) allSubmissions() []*submission {
	return s.history.All()
}

func (s *store) addSubmission(sub *submission) {
	s.history.Add(sub)
}

func (s *store) deleteSubmission(id int) error {
	return s.history.Delete(id)
}

// submissionDB adapts IndexedDB to the browser-independent history store.
type submissionDB struct{}

func (submissionDB) Load(problem string, limit int) ([]*submission, error) {
	rows, err := loadSubmissions(problem, limit)
	logDBErr("讀取", err)
	return rows, err
}

func (submissionDB) LoadAll() ([]*submission, error) {
	rows, err := loadAllSubmissions()
	logDBErr("讀取", err)
	return rows, err
}

func (submissionDB) Save(sub *submission) error {
	err := saveSubmission(sub)
	logDBErr("寫入", err)
	return err
}

func (submissionDB) Delete(id int) error {
	err := deleteSubmission(id)
	logDBErr("刪除", err)
	return err
}

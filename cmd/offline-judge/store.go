//go:build js && wasm

package main

import (
	"sync"
)

// memo caches drafts and submissions, backed by IndexedDB.
// toolgui starts each page with an empty state, so pages read from here.
var memo = &store{text: map[string]string{}, subs: map[string][]*submission{}}

// Submissions kept per problem in memory and shown in history.
const historyLimit = 50

type store struct {
	mu   sync.Mutex
	text map[string]string
	subs map[string][]*submission // by problem, newest first
	all  []*submission            // every problem, newest first; nil until loaded
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

func (s *store) submissions(problem string) []*submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSubs(problem)
}

func (s *store) loadSubs(problem string) []*submission {
	if subs, ok := s.subs[problem]; ok {
		return subs
	}
	subs, err := loadSubmissions(problem, historyLimit)
	logDBErr("讀取", err)
	s.subs[problem] = subs
	return subs
}

// allSubmissions returns the submissions of every problem, newest first.
func (s *store) allSubmissions() []*submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAll()
}

func (s *store) loadAll() []*submission {
	if s.all == nil {
		all, err := loadAllSubmissions()
		logDBErr("讀取", err)
		s.all = append([]*submission{}, all...) // non-nil: loaded
	}
	return s.all
}

func (s *store) addSubmission(sub *submission) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := s.loadSubs(sub.Problem)
	if err := saveSubmission(sub); err != nil {
		logDBErr("寫入", err)
		// Unsaved: number it after the newest so it still shows.
		sub.ID = 1
		if all := s.loadAll(); len(all) > 0 {
			sub.ID = all[0].ID + 1
		}
	}
	subs = append([]*submission{sub}, subs...)
	if len(subs) > historyLimit {
		subs = subs[:historyLimit]
	}
	s.subs[sub.Problem] = subs
	if s.all != nil {
		s.all = append([]*submission{sub}, s.all...)
	}
}

func (s *store) deleteSubmission(problem string, id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleteSubmission(id)
	subs := s.loadSubs(problem)
	for i, sub := range subs {
		if sub.ID == id {
			s.subs[problem] = append(subs[:i:i], subs[i+1:]...)
			break
		}
	}
	for i, sub := range s.all {
		if sub.ID == id {
			s.all = append(s.all[:i:i], s.all[i+1:]...)
			break
		}
	}
}

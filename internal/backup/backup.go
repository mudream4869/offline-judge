// Package backup exports what the app keeps in the browser (drafts,
// settings, contests, submissions) to a file, and merges such a file back
// in without overwriting anything.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mudream4869/offline-judge/internal/contest"
	"github.com/mudream4869/offline-judge/internal/submissions"
)

const (
	app     = "offline-judge"
	version = 1
)

// Data is what a backup holds. The problem cache isn't in it: it can be
// downloaded again.
type Data struct {
	// Drafts are the drafts store: code, custom input, settings, contests.
	Drafts      map[string]string
	Submissions []*submissions.Submission
}

type file struct {
	App         string                    `json:"app"`
	Version     int                       `json:"version"`
	ExportedAt  time.Time                 `json:"exported_at"`
	Drafts      map[string]string         `json:"drafts"`
	Submissions []*submissions.Submission `json:"submissions"`
}

// Encode makes a backup file of d.
func Encode(d Data, now time.Time) ([]byte, error) {
	return json.MarshalIndent(file{App: app, Version: version, ExportedAt: now,
		Drafts: d.Drafts, Submissions: d.Submissions}, "", " ")
}

// Decode reads a backup file made by Encode.
func Decode(bs []byte) (Data, error) {
	var f file
	if err := json.Unmarshal(bs, &f); err != nil {
		return Data{}, fmt.Errorf("不是備份檔：%w", err)
	}
	if f.App != app {
		return Data{}, errors.New("不是 Offline Judge 的備份檔")
	}
	if f.Version > version {
		return Data{}, fmt.Errorf("備份檔的版本 %d 比這個網站新，請先更新網頁", f.Version)
	}
	for _, s := range f.Submissions {
		if s == nil || s.Problem == "" || s.Lang == "" || s.At.IsZero() {
			return Data{}, errors.New("備份檔裡有損壞的提交紀錄")
		}
	}
	return Data{Drafts: f.Drafts, Submissions: f.Submissions}, nil
}

// Keys of drafts that hold lists, merged instead of kept as they are.
const (
	contestsKey = "contests"
	sourcesKey  = "sources"
)

// Merge returns what to write so local has in too: the drafts to set and
// the submissions to add, oldest first. Nothing local is overwritten: a
// draft local has stays, contests and sources are merged, and a submission
// local has (same problem, language, time and code) is skipped.
func Merge(local, in Data) (drafts map[string]string, subs []*submissions.Submission) {
	drafts = map[string]string{}
	for k, v := range in.Drafts {
		old, ok := local.Drafts[k]
		switch {
		case k == contestsKey && ok:
			if m, changed := mergeContests(old, v); changed {
				drafts[k] = m
			}
		case k == sourcesKey && ok:
			if m, changed := mergeList(old, v); changed {
				drafts[k] = m
			}
		case !ok:
			drafts[k] = v
		}
	}

	have := map[subKey]bool{}
	for _, s := range local.Submissions {
		have[keyOf(s)] = true
	}
	for _, s := range in.Submissions {
		if k := keyOf(s); !have[k] {
			have[k] = true
			subs = append(subs, s)
		}
	}
	slices.SortStableFunc(subs, func(a, b *submissions.Submission) int { return a.At.Compare(b.At) })
	return drafts, subs
}

type subKey struct {
	problem, lang, code string
	at                  int64
}

func keyOf(s *submissions.Submission) subKey {
	return subKey{s.Problem, s.Lang, s.Code, s.At.UnixMilli()}
}

// mergeContests adds the contests of in that local lacks, by ID.
func mergeContests(local, in string) (string, bool) {
	cs := contest.Parse(local)
	n := len(cs)
	for _, c := range contest.Parse(in) {
		if !slices.ContainsFunc(cs, func(l contest.Contest) bool { return l.ID == c.ID }) {
			cs = append(cs, c)
		}
	}
	if len(cs) == n {
		return local, false
	}
	slices.SortStableFunc(cs, func(a, b contest.Contest) int { return a.Start.Compare(b.Start) })
	return contest.Marshal(cs), true
}

// mergeList appends the items of the JSON list in that local lacks.
func mergeList(local, in string) (string, bool) {
	var l, add []string
	if json.Unmarshal([]byte(local), &l) != nil || json.Unmarshal([]byte(in), &add) != nil {
		return local, false
	}
	n := len(l)
	for _, v := range add {
		if !slices.Contains(l, v) {
			l = append(l, v)
		}
	}
	if len(l) == n {
		return local, false
	}
	bs, _ := json.Marshal(l)
	return string(bs), true
}

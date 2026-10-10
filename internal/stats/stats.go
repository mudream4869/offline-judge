// Package stats sums up practice from the submission history.
package stats

import (
	"slices"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/submissions"
)

// Problem is a problem of the catalog, for per-tag totals.
type Problem struct {
	ID   string
	Tags []string
}

// Stats is a summary of practice.
type Stats struct {
	Problems    int // in the catalog
	Solved      int // distinct problems with an AC, in the catalog or not
	Listed      int // of Solved, those in the catalog
	Tried       int // submitted to but never AC
	Submissions int
	AC          int // AC submissions
	ActiveDays  int // days with a submission
	Streak      int // days in a row with a submission, ending today or yesterday
	Days        []Day
	Tags        []Tag  // most problems first
	Langs       []Lang // most submissions first
	Verdicts    []Verdict
}

// Day is one day of submissions.
type Day struct {
	Date      time.Time // midnight, local time
	AC, Other int
}

// Tag is how a tag's problems went.
type Tag struct {
	Name          string
	Solved, Total int
}

// Lang is how a language was used.
type Lang struct {
	ID              string
	Submissions, AC int
	Solved          int // distinct problems with an AC in it
}

// Verdict counts submissions with a verdict.
type Verdict struct {
	Verdict judge.Verdict
	Count   int
}

// Compute sums up subs; Days are the last days days up to now.
func Compute(subs []*submissions.Submission, problems []Problem, now time.Time, days int) Stats {
	st := Stats{Problems: len(problems), Submissions: len(subs)}
	solved := map[string]bool{}
	tried := map[string]bool{}
	langs := map[string]*Lang{}
	langSolved := map[string]map[string]bool{}
	verdicts := map[judge.Verdict]int{}
	perDay := map[time.Time]*Day{}

	for _, s := range subs {
		ac := s.Report.Verdict == judge.AC
		tried[s.Problem] = true
		l := langs[s.Lang]
		if l == nil {
			l = &Lang{ID: s.Lang}
			langs[s.Lang] = l
			langSolved[s.Lang] = map[string]bool{}
		}
		l.Submissions++
		verdicts[s.Report.Verdict]++
		d := day(s.At)
		pd := perDay[d]
		if pd == nil {
			pd = &Day{Date: d}
			perDay[d] = pd
		}
		if ac {
			st.AC++
			l.AC++
			solved[s.Problem] = true
			langSolved[s.Lang][s.Problem] = true
			pd.AC++
		} else {
			pd.Other++
		}
	}
	st.Solved = len(solved)
	st.Tried = len(tried) - len(solved)
	st.ActiveDays = len(perDay)

	today := day(now)
	for d := today; ; d = d.AddDate(0, 0, -1) {
		if perDay[d] == nil {
			if d.Equal(today) {
				continue // today may not have started yet
			}
			break
		}
		st.Streak++
	}
	for i := days - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		if pd := perDay[d]; pd != nil {
			st.Days = append(st.Days, *pd)
		} else {
			st.Days = append(st.Days, Day{Date: d})
		}
	}

	tags := map[string]*Tag{}
	for _, p := range problems {
		if solved[p.ID] {
			st.Listed++
		}
		for _, t := range p.Tags {
			tg := tags[t]
			if tg == nil {
				tg = &Tag{Name: t}
				tags[t] = tg
			}
			tg.Total++
			if solved[p.ID] {
				tg.Solved++
			}
		}
	}
	for _, t := range tags {
		st.Tags = append(st.Tags, *t)
	}
	slices.SortFunc(st.Tags, func(a, b Tag) int {
		if a.Total != b.Total {
			return b.Total - a.Total
		}
		return compareStr(a.Name, b.Name)
	})

	for id, l := range langs {
		l.Solved = len(langSolved[id])
		st.Langs = append(st.Langs, *l)
	}
	slices.SortFunc(st.Langs, func(a, b Lang) int {
		if a.Submissions != b.Submissions {
			return b.Submissions - a.Submissions
		}
		return compareStr(a.ID, b.ID)
	})

	for _, v := range []judge.Verdict{judge.AC, judge.WA, judge.TLE, judge.RE, judge.CE, judge.OLE} {
		if n := verdicts[v]; n > 0 {
			st.Verdicts = append(st.Verdicts, Verdict{v, n})
		}
	}
	return st
}

// day is t's midnight in t's location.
func day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func compareStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

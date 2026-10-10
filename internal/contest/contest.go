// Package contest scores mock contests: chosen problems, a time limit, and
// the submissions made in between, ranked ICPC style.
package contest

import (
	"encoding/json"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/submissions"
)

// WrongPenalty is added for each rejected submission before an AC.
const WrongPenalty = 20 * time.Minute

// Contest is one mock contest.
type Contest struct {
	ID       int64 // creation time in ms; unique enough for one user
	Title    string
	Problems []string // problem keys, in order
	Start    time.Time
	End      time.Time // Start + duration, or earlier if ended early
}

// Running reports whether c is on at now.
func (c *Contest) Running(now time.Time) bool {
	return !now.Before(c.Start) && now.Before(c.End)
}

// Has reports whether problem key is in c.
func (c *Contest) Has(key string) bool {
	for _, p := range c.Problems {
		if p == key {
			return true
		}
	}
	return false
}

// Label is the letter of the i-th problem: A, B, …, Z, AA, ….
func Label(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

// Result is how one problem went.
type Result struct {
	Solved bool
	// At is the time from the start to the first AC.
	At time.Duration
	// Wrong counts rejected submissions before the first AC (all of them if
	// unsolved). CE doesn't count, as in ICPC.
	Wrong int
	// Best is the best score, for problems with subtasks; Max is out of.
	Best, Max float64
}

// Standing is the outcome of a contest.
type Standing struct {
	Results []Result // in the order of Contest.Problems
	Solved  int
	// Penalty is the sum, over solved problems, of the time to solve plus
	// WrongPenalty per rejected submission, each rounded down to minutes.
	Penalty time.Duration
}

// Score ranks the submissions to c's problems made while it ran.
func Score(c *Contest, subs []*submissions.Submission) Standing {
	idx := map[string]int{}
	for i, p := range c.Problems {
		idx[p] = i
	}
	st := Standing{Results: make([]Result, len(c.Problems))}
	// Oldest first, so the first AC counts.
	for i := len(subs) - 1; i >= 0; i-- {
		s := subs[i]
		k, ok := idx[s.Problem]
		if !ok || s.At.Before(c.Start) || !s.At.Before(c.End) {
			continue
		}
		r := &st.Results[k]
		r.Best, r.Max = max(r.Best, s.Report.Score), max(r.Max, s.Report.MaxScore)
		switch {
		case r.Solved, s.Report.Verdict == judge.CE:
		case s.Report.Verdict == judge.AC:
			r.Solved, r.At = true, s.At.Sub(c.Start)
		default:
			r.Wrong++
		}
	}
	for _, r := range st.Results {
		if r.Solved {
			st.Solved++
			st.Penalty += r.At.Truncate(time.Minute) + time.Duration(r.Wrong)*WrongPenalty
		}
	}
	return st
}

// Parse reads contests saved by Marshal; bad or empty data is no contests.
func Parse(s string) []Contest {
	var cs []Contest
	if json.Unmarshal([]byte(s), &cs) != nil {
		return nil
	}
	return cs
}

// Marshal saves contests for Parse.
func Marshal(cs []Contest) string {
	bs, _ := json.Marshal(cs)
	return string(bs)
}

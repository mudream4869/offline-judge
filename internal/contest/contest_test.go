package contest

import (
	"testing"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/submissions"
)

func TestLabel(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 51: "AZ", 52: "BA"} {
		if got := Label(i); got != want {
			t.Errorf("Label(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestScore(t *testing.T) {
	t0 := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	c := &Contest{Problems: []string{"a", "b", "c"}, Start: t0, End: t0.Add(time.Hour)}
	sub := func(p string, min float64, v judge.Verdict, score float64) *submissions.Submission {
		at := t0.Add(time.Duration(min * float64(time.Minute)))
		return &submissions.Submission{Problem: p, At: at,
			Report: judge.Report{Verdict: v, Score: score, MaxScore: 100}}
	}
	// Newest first, as the store gives them.
	subs := []*submissions.Submission{
		sub("a", 70, judge.WA, 0),     // after the end
		sub("c", 50, judge.TLE, 40),   // unsolved: no penalty
		sub("a", 30, judge.WA, 0),     // after the AC
		sub("b", 25.9, judge.AC, 100), // 25 min
		sub("a", 12.5, judge.AC, 100), // 12 min + 2 × 20
		sub("a", 11, judge.CE, 0),     // not counted
		sub("a", 10, judge.RE, 0),
		sub("a", 5, judge.WA, 0),
		sub("x", 5, judge.AC, 100),  // not in the contest
		sub("b", -1, judge.AC, 100), // before the start
	}
	st := Score(c, subs)
	if st.Solved != 2 || st.Penalty != (12+40+25)*time.Minute {
		t.Errorf("solved %d, penalty %v; want 2, 77m", st.Solved, st.Penalty)
	}
	a, b, cr := st.Results[0], st.Results[1], st.Results[2]
	if !a.Solved || a.Wrong != 2 || a.At != 12*time.Minute+30*time.Second {
		t.Errorf("a = %+v", a)
	}
	if !b.Solved || b.Wrong != 0 {
		t.Errorf("b = %+v", b)
	}
	if cr.Solved || cr.Wrong != 1 || cr.Best != 40 || cr.Max != 100 {
		t.Errorf("c = %+v", cr)
	}
}

func TestRunningAndParse(t *testing.T) {
	t0 := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	c := Contest{ID: 1, Title: "x", Problems: []string{"a"}, Start: t0, End: t0.Add(time.Hour)}
	if !c.Running(t0) || c.Running(t0.Add(time.Hour)) || c.Running(t0.Add(-time.Second)) {
		t.Error("Running is wrong at the edges")
	}
	cs := Parse(Marshal([]Contest{c}))
	if len(cs) != 1 || !cs[0].Start.Equal(t0) || !cs[0].Has("a") || cs[0].Has("b") {
		t.Errorf("round trip = %+v", cs)
	}
	if Parse("") != nil || Parse("{bad") != nil {
		t.Error("bad data should be no contests")
	}
}

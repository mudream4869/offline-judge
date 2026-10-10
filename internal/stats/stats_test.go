package stats

import (
	"testing"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/submissions"
)

func TestCompute(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, loc)
	at := func(daysAgo, hour int) time.Time {
		return time.Date(2026, 10, 10-daysAgo, hour, 0, 0, 0, loc)
	}
	sub := func(p, lang string, at time.Time, v judge.Verdict) *submissions.Submission {
		return &submissions.Submission{Problem: p, Lang: lang, At: at, Report: judge.Report{Verdict: v}}
	}
	subs := []*submissions.Submission{ // newest first
		sub("a", "py", at(0, 8), judge.AC),
		sub("a", "cpp", at(1, 23), judge.WA),
		sub("b", "py", at(1, 1), judge.AC),
		sub("c", "py", at(2, 12), judge.TLE),
		sub("gone", "go", at(5, 12), judge.AC), // not in the catalog
	}
	problems := []Problem{{"a", []string{"入門"}}, {"b", []string{"入門", "dp"}}, {"c", []string{"dp"}}, {"d", nil}}
	st := Compute(subs, problems, now, 7)

	if st.Problems != 4 || st.Solved != 3 || st.Listed != 2 || st.Tried != 1 || st.Submissions != 5 || st.AC != 3 {
		t.Errorf("totals = %+v", st)
	}
	if st.ActiveDays != 4 || st.Streak != 3 {
		t.Errorf("active days %d, streak %d; want 4, 3", st.ActiveDays, st.Streak)
	}
	if len(st.Days) != 7 || !st.Days[6].Date.Equal(time.Date(2026, 10, 10, 0, 0, 0, 0, loc)) ||
		st.Days[6].AC != 1 || st.Days[5].AC != 1 || st.Days[5].Other != 1 || st.Days[0].AC+st.Days[0].Other != 0 {
		t.Errorf("days = %+v", st.Days)
	}
	want := []Tag{{"dp", 1, 2}, {"入門", 2, 2}}
	if len(st.Tags) != 2 || st.Tags[0] != want[0] || st.Tags[1] != want[1] {
		t.Errorf("tags = %+v", st.Tags)
	}
	if l := st.Langs[0]; l.ID != "py" || l.Submissions != 3 || l.AC != 2 || l.Solved != 2 {
		t.Errorf("langs = %+v", st.Langs)
	}
	if len(st.Verdicts) != 3 || st.Verdicts[0] != (Verdict{judge.AC, 3}) {
		t.Errorf("verdicts = %+v", st.Verdicts)
	}
}

func TestStreakWithoutToday(t *testing.T) {
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	s := func(d int) *submissions.Submission {
		return &submissions.Submission{Problem: "a", At: now.AddDate(0, 0, -d), Report: judge.Report{Verdict: judge.WA}}
	}
	if st := Compute([]*submissions.Submission{s(1), s(2), s(4)}, nil, now, 3); st.Streak != 2 {
		t.Errorf("streak = %d, want 2 (yesterday and the day before)", st.Streak)
	}
	if st := Compute([]*submissions.Submission{s(3)}, nil, now, 3); st.Streak != 0 {
		t.Errorf("streak = %d, want 0", st.Streak)
	}
}

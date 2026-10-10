package backup

import (
	"strings"
	"testing"
	"time"

	"github.com/mudream4869/offline-judge/internal/contest"
	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/submissions"
)

var t0 = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

func sub(id int, problem string, min int, code string) *submissions.Submission {
	return &submissions.Submission{ID: id, Problem: problem, Lang: "py", Code: code,
		At: t0.Add(time.Duration(min) * time.Minute), Report: judge.Report{Verdict: judge.AC}}
}

func TestRoundTrip(t *testing.T) {
	d := Data{Drafts: map[string]string{"code_py_a": "print(1)"},
		Submissions: []*submissions.Submission{sub(3, "a", 1, "x")}}
	bs, err := Encode(d, t0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(bs)
	if err != nil {
		t.Fatal(err)
	}
	s := got.Submissions[0]
	if got.Drafts["code_py_a"] != "print(1)" || s.ID != 3 || !s.At.Equal(t0.Add(time.Minute)) ||
		s.Report.Verdict != judge.AC {
		t.Errorf("round trip = %+v, %+v", got, s)
	}
}

func TestDecodeRejects(t *testing.T) {
	for name, in := range map[string]string{
		"not json":   "hello",
		"other app":  `{"app": "x", "version": 1}`,
		"newer":      `{"app": "offline-judge", "version": 99}`,
		"broken sub": `{"app": "offline-judge", "version": 1, "submissions": [{"Problem": "a"}]}`,
	} {
		if _, err := Decode([]byte(in)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestMerge(t *testing.T) {
	c := func(id int64, min int) contest.Contest {
		return contest.Contest{ID: id, Title: "c", Start: t0.Add(time.Duration(min) * time.Minute)}
	}
	local := Data{
		Drafts: map[string]string{
			"code_py_a": "mine",
			"sources":   `["s1"]`,
			"contests":  contest.Marshal([]contest.Contest{c(2, 20)}),
		},
		Submissions: []*submissions.Submission{sub(1, "a", 5, "x")},
	}
	in := Data{
		Drafts: map[string]string{
			"code_py_a": "theirs",                                               // kept local
			"code_py_b": "new",                                                  // added
			"sources":   `["s2", "s1"]`,                                         // union
			"contests":  contest.Marshal([]contest.Contest{c(1, 10), c(2, 20)}), // one new
		},
		Submissions: []*submissions.Submission{
			sub(9, "a", 7, "z"),
			sub(7, "a", 5, "x"), // same as local
			sub(8, "b", 6, "y"),
		},
	}
	drafts, subs := Merge(local, in)
	if _, ok := drafts["code_py_a"]; ok || drafts["code_py_b"] != "new" || drafts["sources"] != `["s1","s2"]` {
		t.Errorf("drafts = %v", drafts)
	}
	cs := contest.Parse(drafts["contests"])
	if len(cs) != 2 || cs[0].ID != 1 || cs[1].ID != 2 {
		t.Errorf("contests = %+v", cs)
	}
	if len(subs) != 2 || subs[0].Problem != "b" || subs[1].Code != "z" {
		t.Errorf("subs = %v", subs)
	}

	// Merging again adds nothing.
	local.Drafts["code_py_b"], local.Drafts["sources"], local.Drafts["contests"] =
		drafts["code_py_b"], drafts["sources"], drafts["contests"]
	local.Submissions = append(local.Submissions, subs...)
	if drafts, subs := Merge(local, in); len(drafts) != 0 || len(subs) != 0 {
		t.Errorf("second merge = %v, %v", drafts, subs)
	}
	if !strings.Contains(string(must(Encode(local, t0))), `"app": "offline-judge"`) {
		t.Error("no app marker")
	}
}

func must(bs []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return bs
}

package judge

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestEqual(t *testing.T) {
	tests := []struct {
		got, want string
		eq        bool
	}{
		{"3\n", "3\n", true},
		{"3", "3\n", true},
		{"3  \n\n\n", "3\n", true},
		{"1 2\r\n3\r\n", "1 2\n3\n", true},
		{"\n3\n", "3\n", false},
		{"1  2\n", "1 2\n", false},
		{"4\n", "3\n", false},
	}
	for _, tt := range tests {
		if got := Equal(tt.got, tt.want); got != tt.eq {
			t.Errorf("Equal(%q, %q) = %v, want %v", tt.got, tt.want, got, tt.eq)
		}
	}
}

// fakeRunner answers each input from a table.
type fakeRunner map[string]RunResult

func (f fakeRunner) Run(_ context.Context, _ string, in Input, _ time.Duration) (RunResult, error) {
	res, ok := f[in.Stdin]
	if !ok {
		return RunResult{}, errors.New("no such input")
	}
	return res, nil
}

func TestJudge(t *testing.T) {
	r := fakeRunner{
		"a": {Status: RunOK, Stdout: "1\n", Time: time.Millisecond},
		"b": {Status: RunOK, Stdout: "x\n", Time: time.Millisecond},
		"c": {Status: RunError, Stderr: "boom"},
		"d": {Status: RunOK, Stdout: "1\n", Time: 2 * time.Second},
		"e": {Status: RunTimeout},
	}
	cases := []Case{
		{Name: "a", Input: "a", Output: "1"},
		{Name: "b", Input: "b", Output: "1"},
		{Name: "c", Input: "c", Output: "1"},
		{Name: "d", Input: "d", Output: "1"},
		{Name: "e", Input: "e", Output: "1"},
	}

	calls := 0
	rep, err := Judge(context.Background(), r, "", cases, Spec{Limit: time.Second},
		func(int, CaseResult) { calls++ })
	if err != nil {
		t.Fatal(err)
	}

	want := []Verdict{AC, WA, RE, TLE, Skip}
	for i, cr := range rep.Cases {
		if cr.Verdict != want[i] {
			t.Errorf("case %s: got %s, want %s", cr.Name, cr.Verdict, want[i])
		}
	}
	if rep.Verdict != WA {
		t.Errorf("overall: got %s, want WA", rep.Verdict)
	}
	if calls != 4 {
		t.Errorf("progress called %d times, want 4", calls)
	}
}

func TestJudgeAllAC(t *testing.T) {
	r := fakeRunner{"a": {Status: RunOK, Stdout: "1"}}
	rep, err := Judge(context.Background(), r, "", []Case{{Input: "a", Output: "1\n"}},
		Spec{Limit: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != AC {
		t.Errorf("got %s, want AC", rep.Verdict)
	}
}

func TestJudgeCE(t *testing.T) {
	r := fakeRunner{"a": {Status: RunCompileError, Stderr: "error: x"}}
	rep, err := Judge(context.Background(), r, "", []Case{{Input: "a"}, {Input: "b"}},
		Spec{Limit: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != CE || rep.CompileError != "error: x" || len(rep.Cases) != 0 {
		t.Errorf("got %+v, want CE with no cases", rep)
	}
}

func TestJudgeChecker(t *testing.T) {
	r := fakeRunner{
		"a": {Status: RunOK, Stdout: "2 1\n"},
		"b": {Status: RunOK, Stdout: "3\n"},
	}
	// Accepts any order of the expected numbers.
	check := func(_ context.Context, c Case, out string) (bool, string, error) {
		if len(out) != len(c.Output) {
			return false, "length differs", nil
		}
		return true, "", nil
	}
	cases := []Case{{Name: "a", Input: "a", Output: "1 2\n"}, {Name: "b", Input: "b", Output: "1 2\n"}}
	rep, err := Judge(context.Background(), r, "", cases, Spec{Limit: time.Second, Check: check}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != WA || rep.Cases[0].Verdict != AC || rep.Cases[1].Message != "length differs" {
		t.Errorf("got %+v", rep)
	}

	boom := func(context.Context, Case, string) (bool, string, error) {
		return false, "", errors.New("boom")
	}
	if _, err := Judge(context.Background(), r, "", cases, Spec{Limit: time.Second, Check: boom}, nil); err == nil {
		t.Error("checker error not returned")
	}
}

func TestJudgeInteractive(t *testing.T) {
	r := fakeRunner{
		"ac":    {Status: RunOK, Judged: &Judgement{OK: true}},
		"wa":    {Status: RunOK, Judged: &Judgement{Message: "wrong guess"}},
		"cut":   {Status: RunError, Judged: &Judgement{Message: "too many queries"}},
		"crash": {Status: RunError, Judged: &Judgement{OK: true}},
		"slow":  {Status: RunTimeout},
	}
	cases := []Case{{Input: "ac"}, {Input: "wa"}, {Input: "cut"}, {Input: "crash"}, {Input: "slow"}}
	rep, err := Judge(context.Background(), r, "", cases,
		Spec{Limit: time.Second, Interactor: "x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Verdict{AC, WA, WA, RE, TLE}
	for i, cr := range rep.Cases {
		if cr.Verdict != want[i] {
			t.Errorf("case %s: got %s, want %s", cases[i].Input, cr.Verdict, want[i])
		}
	}
	if rep.Cases[2].Message != "too many queries" {
		t.Errorf("message = %q", rep.Cases[2].Message)
	}

	r["none"] = RunResult{Status: RunOK}
	if _, err := Judge(context.Background(), r, "", []Case{{Input: "none"}},
		Spec{Limit: time.Second, Interactor: "x"}, nil); err == nil {
		t.Error("missing verdict not reported")
	}
}

// countRunner counts runs of each input.
type countRunner struct {
	fakeRunner
	runs map[string]int
}

func (c *countRunner) Run(ctx context.Context, code string, in Input, limit time.Duration) (RunResult, error) {
	c.runs[in.Stdin]++
	return c.fakeRunner.Run(ctx, code, in, limit)
}

func TestJudgeSubtasks(t *testing.T) {
	ok := RunResult{Status: RunOK, Stdout: "1"}
	r := &countRunner{fakeRunner: fakeRunner{
		"s":  ok,
		"a1": ok, "a2": ok,
		"b1": {Status: RunTimeout}, "b2": ok,
		"c1": ok, "c2": {Status: RunOK, Stdout: "2"},
	}, runs: map[string]int{}}
	var cases []Case
	for _, n := range []string{"s", "a1", "a2", "b1", "b2", "c1", "c2"} {
		cases = append(cases, Case{Name: n, Input: n, Output: "1"})
	}
	spec := Spec{Limit: time.Second, Subtasks: []Subtask{
		{Score: 20, Cases: []string{"a1", "a2"}},
		{Score: 30, Cases: []string{"a1", "a2", "b1", "b2"}}, // b1 TLE: b2 is skipped
		{Score: 50, Cases: []string{"b2", "c1", "c2"}},       // still runs b2 for this one
	}}
	rep, err := Judge(context.Background(), r, "", cases, spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Verdict{AC, AC, AC, TLE, AC, AC, WA}
	for i, cr := range rep.Cases {
		if cr.Verdict != want[i] {
			t.Errorf("case %s: got %s, want %s", cr.Name, cr.Verdict, want[i])
		}
	}
	if rep.Verdict != TLE || rep.Score != 20 || rep.MaxScore != 100 {
		t.Errorf("got %s %v/%v, want TLE 20/100", rep.Verdict, rep.Score, rep.MaxScore)
	}
	wantSubs := []SubtaskResult{{20, 20, AC}, {0, 30, TLE}, {0, 50, WA}}
	if !slices.Equal(rep.Subtasks, wantSubs) {
		t.Errorf("subtasks = %+v, want %+v", rep.Subtasks, wantSubs)
	}

	// Once b2's only subtask fails, it is skipped.
	spec.Subtasks = spec.Subtasks[:2]
	rep, err = Judge(context.Background(), r, "", cases, spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Cases[4].Verdict != Skip || rep.Cases[5].Verdict != AC {
		t.Errorf("got %+v, want b2 skipped and c1 (in no subtask) run", rep.Cases)
	}
	if r.runs["b2"] != 1 {
		t.Errorf("b2 ran %d times, want 1 (only in the first judge)", r.runs["b2"])
	}
	if rep.Subtasks[1].Verdict != TLE {
		t.Errorf("subtask 2: got %s, want TLE (not SKIP)", rep.Subtasks[1].Verdict)
	}
}

func TestJudgeSubtasksCE(t *testing.T) {
	r := fakeRunner{"a": {Status: RunCompileError}}
	rep, err := Judge(context.Background(), r, "", []Case{{Name: "a", Input: "a"}},
		Spec{Limit: time.Second, Subtasks: []Subtask{{Score: 100, Cases: []string{"a"}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != CE || rep.Score != 0 || rep.MaxScore != 100 {
		t.Errorf("got %+v, want CE 0/100", rep)
	}
}

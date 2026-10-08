package judge

import (
	"context"
	"errors"
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

func (f fakeRunner) Run(_ context.Context, _, stdin string, _ time.Duration) (RunResult, error) {
	res, ok := f[stdin]
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
	rep, err := Judge(context.Background(), r, "", cases, time.Second, nil,
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
		time.Second, nil, nil)
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
		time.Second, nil, nil)
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
	rep, err := Judge(context.Background(), r, "", cases, time.Second, check, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != WA || rep.Cases[0].Verdict != AC || rep.Cases[1].Message != "length differs" {
		t.Errorf("got %+v", rep)
	}

	boom := func(context.Context, Case, string) (bool, string, error) {
		return false, "", errors.New("boom")
	}
	if _, err := Judge(context.Background(), r, "", cases, time.Second, boom, nil); err == nil {
		t.Error("checker error not returned")
	}
}

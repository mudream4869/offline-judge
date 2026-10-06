// Package judge runs a submission against test cases and gives verdicts.
// It knows nothing about the browser: the language runtime is a Runner.
package judge

import (
	"context"
	"strings"
	"time"
)

// Verdict is the outcome of one test case, or of a whole submission.
type Verdict string

const (
	AC  Verdict = "AC"
	WA  Verdict = "WA"
	TLE Verdict = "TLE"
	RE  Verdict = "RE"
	CE  Verdict = "CE"
	// Skip marks cases not run after a TLE: each TLE costs a runtime restart.
	Skip Verdict = "SKIP"
)

// RunStatus is how a single run ended, before its output is checked.
type RunStatus string

const (
	RunOK      RunStatus = "ok"
	RunError   RunStatus = "re"
	RunTimeout RunStatus = "tle"
	// RunCompileError: Stderr holds the compiler output.
	RunCompileError RunStatus = "ce"
)

// RunResult is what a Runner reports for one execution.
type RunResult struct {
	Status RunStatus
	Stdout string
	Stderr string
	Time   time.Duration
}

// Runner executes code with stdin under a time limit.
// It must stop the program once limit is clearly exceeded.
type Runner interface {
	Run(ctx context.Context, code, stdin string, limit time.Duration) (RunResult, error)
}

// Case is one test case.
type Case struct {
	Name   string
	Input  string
	Output string
}

// CaseResult is the verdict of one test case.
type CaseResult struct {
	Name    string
	Verdict Verdict
	Time    time.Duration
	Stdout  string
	Stderr  string
}

// Report is the result of judging a submission.
type Report struct {
	Verdict Verdict
	Cases   []CaseResult
	// CompileError is the compiler output when Verdict is CE.
	CompileError string
}

// Judge runs code on each case, stopping after the first TLE or a CE.
// The overall verdict is the first non-AC one.
// progress, if not nil, is called after each case.
func Judge(ctx context.Context, r Runner, code string, cases []Case,
	limit time.Duration, progress func(done int, cr CaseResult)) (Report, error) {

	rep := Report{Verdict: AC}
	for i, c := range cases {
		res, err := r.Run(ctx, code, c.Input, limit)
		if err != nil {
			return rep, err
		}
		if res.Status == RunCompileError {
			return Report{Verdict: CE, CompileError: res.Stderr}, nil
		}

		cr := CaseResult{
			Name:   c.Name,
			Time:   res.Time,
			Stdout: res.Stdout,
			Stderr: res.Stderr,
		}
		switch {
		case res.Status == RunTimeout || res.Time > limit:
			cr.Verdict = TLE
		case res.Status == RunError:
			cr.Verdict = RE
		case Equal(res.Stdout, c.Output):
			cr.Verdict = AC
		default:
			cr.Verdict = WA
		}

		if rep.Verdict == AC && cr.Verdict != AC {
			rep.Verdict = cr.Verdict
		}
		rep.Cases = append(rep.Cases, cr)

		if progress != nil {
			progress(i+1, cr)
		}

		if cr.Verdict == TLE {
			for _, rest := range cases[i+1:] {
				rep.Cases = append(rep.Cases, CaseResult{Name: rest.Name, Verdict: Skip})
			}
			break
		}
	}

	return rep, nil
}

// Equal compares outputs ignoring trailing spaces and trailing blank lines.
func Equal(got, want string) bool {
	return normalize(got) == normalize(want)
}

func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// Package judge runs a submission against test cases and gives verdicts.
// It knows nothing about the browser: the language runtime is a Runner.
package judge

import (
	"context"
	"errors"
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
	Stdout string // with an interactor, the transcript
	Stderr string
	Time   time.Duration
	// Judged is the interactor's verdict, once the program ended in time.
	Judged *Judgement
}

// Judgement is an interactor's verdict.
type Judgement struct {
	OK      bool
	Message string
}

// Input is what a program reads.
type Input struct {
	Stdin string
	// Interactor is interactor.js. When set, the program reads what it
	// answers instead, and Stdin is the interactor's input.
	Interactor string
}

// Runner executes code under a time limit.
// It must stop the program once limit is clearly exceeded.
type Runner interface {
	Run(ctx context.Context, code string, in Input, limit time.Duration) (RunResult, error)
}

// Checker decides whether output answers c. msg explains a rejection.
// An error means the checker itself failed.
type Checker func(ctx context.Context, c Case, output string) (ok bool, msg string, err error)

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
	Message string `json:",omitempty"` // from the checker or interactor
}

// Report is the result of judging a submission.
type Report struct {
	Verdict Verdict
	Cases   []CaseResult
	// CompileError is the compiler output when Verdict is CE.
	CompileError string
}

// Spec is how a problem is judged.
type Spec struct {
	Limit time.Duration
	Check Checker // compares outputs; nil means Equal
	// Interactor is interactor.js for an interactive problem: it talks to
	// the program, with each case's Input as its input, and gives the verdict.
	Interactor string
}

// Judge runs code on each case, stopping after the first TLE or a CE.
// The overall verdict is the first non-AC one.
// progress, if not nil, is called after each case.
func Judge(ctx context.Context, r Runner, code string, cases []Case,
	spec Spec, progress func(done int, cr CaseResult)) (Report, error) {

	limit, check := spec.Limit, spec.Check
	if check == nil {
		check = func(_ context.Context, c Case, out string) (bool, string, error) {
			return Equal(out, c.Output), "", nil
		}
	}

	rep := Report{Verdict: AC}
	for i, c := range cases {
		res, err := r.Run(ctx, code, Input{Stdin: c.Input, Interactor: spec.Interactor}, limit)
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
		case res.Judged != nil && !res.Judged.OK:
			// Before RE: a program cut off by the interactor often crashes.
			cr.Verdict, cr.Message = WA, res.Judged.Message
		case res.Status == RunError:
			cr.Verdict = RE
		case spec.Interactor != "":
			if res.Judged == nil {
				return rep, errors.New("互動程式沒有回報結果")
			}
			cr.Verdict = AC
		default:
			ok, msg, err := check(ctx, c, res.Stdout)
			if err != nil {
				return rep, err
			}
			cr.Verdict, cr.Message = WA, msg
			if ok {
				cr.Verdict = AC
			}
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

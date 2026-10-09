// Package judge runs a submission against test cases and gives verdicts.
// It knows nothing about the browser: the language runtime is a Runner.
package judge

import (
	"context"
	"errors"
	"slices"
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
	OLE Verdict = "OLE"
	// Skip marks cases not run: after a TLE, since each TLE costs a runtime
	// restart; with subtasks, ones whose subtasks have all failed.
	Skip Verdict = "SKIP"
)

// RunStatus is how a single run ended, before its output is checked.
type RunStatus string

const (
	RunOK      RunStatus = "ok"
	RunError   RunStatus = "re"
	RunTimeout RunStatus = "tle"
	// RunOutputLimit: the program was stopped for writing too much.
	RunOutputLimit RunStatus = "ole"
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
	// With subtasks: the score out of MaxScore, and each subtask's result.
	Score    float64         `json:",omitempty"`
	MaxScore float64         `json:",omitempty"`
	Subtasks []SubtaskResult `json:",omitempty"`
}

// Subtask is a group of cases worth Score if all of them pass.
type Subtask struct {
	Score float64
	Cases []string // case names
}

// SubtaskResult is the result of one subtask.
type SubtaskResult struct {
	Score, Max float64
	Verdict    Verdict // AC, or its first case's non-AC verdict
}

// Spec is how a problem is judged.
type Spec struct {
	Limit time.Duration
	Check Checker // compares outputs; nil means Compare
	// Compare is the built-in comparison used without Check.
	Compare Compare
	// Interactor is interactor.js for an interactive problem: it talks to
	// the program, with each case's Input as its input, and gives the verdict.
	Interactor string
	// Subtasks score the cases. Without them a TLE skips the rest; with
	// them, a case is skipped once every subtask it is in has failed.
	Subtasks []Subtask
}

// Judge runs code on each case, stopping at a CE. The overall verdict is
// the first non-AC one; see Spec.Subtasks for skipped cases.
// progress, if not nil, is called after each case.
func Judge(ctx context.Context, r Runner, code string, cases []Case,
	spec Spec, progress func(done int, cr CaseResult)) (Report, error) {

	limit, check := spec.Limit, spec.Check
	if check == nil {
		check = func(_ context.Context, c Case, out string) (bool, string, error) {
			ok, msg := spec.Compare.Check(out, c.Output)
			return ok, msg, nil
		}
	}

	rep := Report{Verdict: AC}
	inSub := map[string][]int{} // case name → subtasks it is in
	for i, st := range spec.Subtasks {
		rep.MaxScore += st.Score
		for _, name := range st.Cases {
			inSub[name] = append(inSub[name], i)
		}
	}
	failed := make([]bool, len(spec.Subtasks))
	// skip reports whether c can no longer change the score.
	skip := func(c Case) bool {
		subs := inSub[c.Name]
		for _, i := range subs {
			if !failed[i] {
				return false
			}
		}
		return len(subs) > 0
	}

	for i, c := range cases {
		if skip(c) {
			rep.Cases = append(rep.Cases, CaseResult{Name: c.Name, Verdict: Skip})
			continue
		}
		res, err := r.Run(ctx, code, Input{Stdin: c.Input, Interactor: spec.Interactor}, limit)
		if err != nil {
			return rep, err
		}
		if res.Status == RunCompileError {
			return Report{Verdict: CE, CompileError: res.Stderr, MaxScore: rep.MaxScore}, nil
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
		case res.Status == RunOutputLimit:
			cr.Verdict = OLE
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

		if cr.Verdict != AC {
			if rep.Verdict == AC {
				rep.Verdict = cr.Verdict
			}
			for _, j := range inSub[c.Name] {
				failed[j] = true
			}
		}
		rep.Cases = append(rep.Cases, cr)

		if progress != nil {
			progress(i+1, cr)
		}

		if cr.Verdict == TLE && len(spec.Subtasks) == 0 {
			for _, rest := range cases[i+1:] {
				rep.Cases = append(rep.Cases, CaseResult{Name: rest.Name, Verdict: Skip})
			}
			break
		}
	}

	scoreSubtasks(&rep, spec.Subtasks)
	return rep, nil
}

// scoreSubtasks fills in rep's score from its cases.
func scoreSubtasks(rep *Report, subs []Subtask) {
	for _, st := range subs {
		r := SubtaskResult{Max: st.Score, Verdict: AC}
		// In case order, so a Skip never hides the failure that caused it.
		for _, cr := range rep.Cases {
			if slices.Contains(st.Cases, cr.Name) && cr.Verdict != AC {
				r.Verdict = cr.Verdict
				break
			}
		}
		if r.Verdict == AC {
			r.Score = st.Score
			rep.Score += st.Score
		}
		rep.Subtasks = append(rep.Subtasks, r)
	}
}

// Equal compares outputs ignoring trailing spaces and trailing blank lines.
func Equal(got, want string) bool {
	ok, _ := checkLines(got, want)
	return ok
}

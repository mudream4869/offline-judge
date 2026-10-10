package judge

import (
	"context"
	"strconv"
	"time"
)

// StressSpec is a stress test: a generator makes inputs from seeds, and a
// slow but sure solution checks the code under test on each.
type StressSpec struct {
	Gen, Brute, Code string
	// Grader is the main program of Brute and Code; "" if none.
	Grader string
	Limit  time.Duration // for Code
	// SlowLimit is for Gen and Brute, which may be slow.
	SlowLimit time.Duration
	Check     Checker // nil means Compare, with Brute's output as the answer
	Compare   Compare
}

// StressFailure is the first round that failed.
type StressFailure struct {
	Seed      int
	Input     string
	Want, Got string // Brute's and Code's outputs
	// Who failed to run: "gen", "brute" or "code"; "" if Code's output
	// was rejected.
	Who     string
	Verdict Verdict // of Who, or WA
	Stderr  string  // of Who
	Message string  // why the output was rejected
}

// Stress runs rounds rounds from seed first, stopping at the first failure,
// which it returns; nil means every round passed. progress, if not nil, is
// called after each round.
func Stress(ctx context.Context, r Runner, s StressSpec, first, rounds int,
	progress func(done int)) (*StressFailure, error) {

	check := s.Check
	if check == nil {
		check = func(_ context.Context, c Case, out string) (bool, string, error) {
			ok, msg := s.Compare.Check(out, c.Output)
			return ok, msg, nil
		}
	}
	// run runs one program; a failure is returned as a StressFailure.
	run := func(who, code string, in Input, limit time.Duration,
		f *StressFailure) (string, *StressFailure, error) {

		res, err := r.Run(ctx, code, in, limit)
		if err != nil {
			return "", nil, err
		}
		var v Verdict
		switch {
		case res.Status == RunCompileError:
			v = CE
		case res.Status == RunTimeout || res.Time > limit:
			v = TLE
		case res.Status == RunOutputLimit:
			v = OLE
		case res.Status == RunError:
			v = RE
		default:
			return res.Stdout, nil, nil
		}
		f.Who, f.Verdict, f.Stderr = who, v, res.Stderr
		if who == "code" {
			f.Got = res.Stdout
		}
		return "", f, nil
	}

	for i := range rounds {
		seed := first + i
		f := &StressFailure{Seed: seed}
		in, fail, err := run("gen", s.Gen, Input{Stdin: strconv.Itoa(seed) + "\n"}, s.SlowLimit, f)
		if err != nil || fail != nil {
			return fail, err
		}
		f.Input = in
		want, fail, err := run("brute", s.Brute, Input{Stdin: in, Grader: s.Grader}, s.SlowLimit, f)
		if err != nil || fail != nil {
			return fail, err
		}
		f.Want = want
		got, fail, err := run("code", s.Code, Input{Stdin: in, Grader: s.Grader}, s.Limit, f)
		if err != nil || fail != nil {
			return fail, err
		}
		f.Got = got
		ok, msg, err := check(ctx, Case{Name: "seed " + strconv.Itoa(seed), Input: in, Output: want}, got)
		if err != nil {
			return nil, err
		}
		if !ok {
			f.Verdict, f.Message = WA, msg
			return f, nil
		}
		if progress != nil {
			progress(i + 1)
		}
	}
	return nil, nil
}

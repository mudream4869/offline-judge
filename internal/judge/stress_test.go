package judge

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

// funcRunner runs "programs" that are Go functions, by name.
type funcRunner map[string]func(stdin string) (string, RunStatus)

func (f funcRunner) Run(_ context.Context, code string, in Input, _ time.Duration) (RunResult, error) {
	out, st := f[code](in.Stdin)
	return RunResult{Status: st, Stdout: out}, nil
}

func TestStress(t *testing.T) {
	progs := funcRunner{
		"gen": func(seed string) (string, RunStatus) { // n = seed
			n, _ := strconv.Atoi(strings.TrimSpace(seed))
			return strconv.Itoa(n) + "\n", RunOK
		},
		"brute": func(in string) (string, RunStatus) { // sum 1..n
			n, _ := strconv.Atoi(strings.TrimSpace(in))
			s := 0
			for i := 1; i <= n; i++ {
				s += i
			}
			return strconv.Itoa(s) + "\n", RunOK
		},
		"good": func(in string) (string, RunStatus) {
			n, _ := strconv.Atoi(strings.TrimSpace(in))
			return strconv.Itoa(n*(n+1)/2) + "\n", RunOK
		},
		"bad": func(in string) (string, RunStatus) { // wrong from n = 7
			n, _ := strconv.Atoi(strings.TrimSpace(in))
			if n >= 7 {
				n--
			}
			return strconv.Itoa(n*(n+1)/2) + "\n", RunOK
		},
		"crash": func(string) (string, RunStatus) { return "", RunError },
	}
	spec := func(code, brute string) StressSpec {
		return StressSpec{Gen: "gen", Brute: brute, Code: code, Limit: time.Second, SlowLimit: time.Second}
	}
	ctx := context.Background()

	done := 0
	f, err := Stress(ctx, progs, spec("good", "brute"), 1, 20, func(n int) { done = n })
	if err != nil || f != nil || done != 20 {
		t.Errorf("good: %+v, %v, %d rounds", f, err, done)
	}
	f, _ = Stress(ctx, progs, spec("bad", "brute"), 1, 20, nil)
	if f == nil || f.Seed != 7 || f.Who != "" || f.Verdict != WA || f.Want != "28\n" || f.Got != "21\n" ||
		f.Input != "7\n" || f.Message == "" {
		t.Errorf("bad: %+v", f)
	}
	f, _ = Stress(ctx, progs, spec("crash", "brute"), 3, 5, nil)
	if f == nil || f.Seed != 3 || f.Who != "code" || f.Verdict != RE || f.Want != "6\n" {
		t.Errorf("crash: %+v", f)
	}
	f, _ = Stress(ctx, progs, spec("good", "crash"), 1, 5, nil)
	if f == nil || f.Who != "brute" || f.Verdict != RE {
		t.Errorf("brute crash: %+v", f)
	}
	// A checker sees brute's output as the answer.
	sp := spec("bad", "brute")
	sp.Check = func(_ context.Context, c Case, out string) (bool, string, error) { return true, "", nil }
	if f, _ := Stress(ctx, progs, sp, 1, 20, nil); f != nil {
		t.Errorf("checker accepting everything: %+v", f)
	}
}

// Command benchjudge applies the site's judge to one benchmark worker result.
// It reads one JSON request from stdin and writes one JSON response to stdout.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

type request struct {
	Compare     string         `json:"compare"`
	Interactive bool           `json:"interactive"`
	Answer      string         `json:"answer"`
	Result      workerResult   `json:"result"`
	Checker     *checkerResult `json:"checker,omitempty"`
}

type workerResult struct {
	Status  judge.RunStatus  `json:"status"`
	Stdout  string           `json:"stdout"`
	Stderr  string           `json:"stderr"`
	Judged  *judge.Judgement `json:"judged"`
	IAError string           `json:"iaError"`
}

type checkerResult struct {
	Value any `json:"value"`
}

type response struct {
	Verdict judge.Verdict `json:"verdict,omitempty"`
	Error   string        `json:"error,omitempty"`
}

type resultRunner struct{ result judge.RunResult }

func (r resultRunner) Run(context.Context, string, judge.Input, time.Duration) (judge.RunResult, error) {
	return r.result, nil
}

func evaluate(req request) response {
	cm, err := judge.ParseCompare(req.Compare)
	if err != nil {
		return response{Error: err.Error()}
	}
	// Match pool.run: interactor errors are infrastructure errors, not WA/RE.
	if req.Result.IAError != "" {
		return response{Error: "互動程式錯誤：" + req.Result.IAError}
	}
	spec := judge.Spec{Compare: cm}
	if req.Interactive {
		spec.Interactor = "interactor.js"
	}
	if req.Checker != nil {
		spec.Check = func(context.Context, judge.Case, string) (bool, string, error) {
			switch v := req.Checker.Value.(type) {
			case bool:
				return v, "", nil
			case string:
				return false, v, nil
			default:
				return false, "", errors.New("checker 應回傳 true / false / 字串")
			}
		}
	}
	// The benchmark measures beyond the problem's limit to calibrate it.
	// Leave Time at zero: only a worker killed at --cap reports RunTimeout.
	r := resultRunner{judge.RunResult{
		Status: req.Result.Status,
		Stdout: req.Result.Stdout,
		Stderr: req.Result.Stderr,
		Judged: req.Result.Judged,
	}}
	rep, err := judge.Judge(context.Background(), r, "", []judge.Case{{Output: req.Answer}}, spec, nil)
	if err != nil {
		return response{Error: err.Error()}
	}
	return response{Verdict: rep.Verdict}
}

func serve(in io.Reader, out io.Writer) error {
	var req request
	if err := json.NewDecoder(in).Decode(&req); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(evaluate(req))
}

func main() {
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

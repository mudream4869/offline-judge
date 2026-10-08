//go:build js && wasm

package main

import (
	"context"
	"errors"
	"sync"
	"syscall/js"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

const compileTimeout = time.Minute

var errCompileTimeout = errors.New("編譯逾時")

// CppRunner implements judge.Runner: clang in a long-lived worker, programs
// in a killable pool.
type CppRunner struct {
	ccURL string
	exec  *pool

	mu  sync.Mutex
	cc  *worker
	seq int
	// Last compile, reused by every case of a submission.
	code   string
	module js.Value // undefined on CE
	ce     string
}

func NewCppRunner(ccURL, runURL string) *CppRunner {
	return &CppRunner{
		ccURL: ccURL,
		cc:    spawnWorker(ccURL),
		exec:  newPool(runURL, "C++"),
	}
}

// Ready reports whether clang has loaded.
func (r *CppRunner) Ready() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cc.isReady()
}

func (r *CppRunner) compile(ctx context.Context, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.code == code && (!r.module.IsUndefined() || r.ce != "") {
		return nil
	}

	if err := r.cc.wait(ctx, "C++"); err != nil {
		if r.cc.loadErr != nil {
			r.cc.kill()
			r.cc = spawnWorker(r.ccURL)
		}
		return err
	}

	msg := js.Global().Get("Object").New()
	msg.Set("code", code)
	r.seq++
	data, err := r.cc.post(ctx, r.seq, msg, time.After(compileTimeout))
	if err == errTimeout {
		// Probably stuck: restart clang.
		r.cc.kill()
		r.cc = spawnWorker(r.ccURL)
		return errCompileTimeout
	}
	if err != nil {
		return err
	}

	r.code = code
	r.module = js.Undefined()
	r.ce = ""
	if data.Get("status").String() == "ok" {
		r.module = data.Get("module")
	} else {
		r.ce = data.Get("stderr").String()
	}
	return nil
}

func (r *CppRunner) Run(ctx context.Context, code string, in judge.Input,
	limit time.Duration) (judge.RunResult, error) {

	if err := r.compile(ctx, code); err != nil {
		return judge.RunResult{}, err
	}

	r.mu.Lock()
	module, ce := r.module, r.ce
	r.mu.Unlock()
	if module.IsUndefined() {
		return judge.RunResult{Status: judge.RunCompileError, Stderr: ce}, nil
	}

	msg := js.Global().Get("Object").New()
	msg.Set("module", module)
	setInput(msg, in)
	return r.exec.run(ctx, msg, limit)
}

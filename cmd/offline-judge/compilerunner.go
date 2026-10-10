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

// CompileRunner implements judge.Runner for compiled languages: the
// compiler in a long-lived worker, programs in a killable pool.
type CompileRunner struct {
	name  string
	ccURL string
	exec  *pool

	mu  sync.Mutex
	cc  *worker
	seq int
	// Recent compiles, oldest first: every case of a submission reuses
	// one, and a stress test takes turns between three.
	cache []compiled
}

// compiled is a compile of code with grader.
type compiled struct {
	code, grader string
	module       js.Value // undefined on CE
	ce           string
}

// compileCache is how many compiles CompileRunner keeps.
const compileCache = 4

func NewCompileRunner(name, ccURL, runURL string) *CompileRunner {
	return &CompileRunner{
		name:  name,
		ccURL: ccURL,
		cc:    spawnWorker(ccURL),
		exec:  newPool(runURL, name),
	}
}

// Ready reports whether the compiler has loaded.
func (r *CompileRunner) Ready() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cc.isReady()
}

func (r *CompileRunner) awaitLoad() {
	r.mu.Lock()
	cc := r.cc
	r.mu.Unlock()
	cc.await()
}

// compile returns the compile of code with grader, from the cache if there.
func (r *CompileRunner) compile(ctx context.Context, code, grader string) (compiled, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.cache {
		if c.code == code && c.grader == grader {
			// Most recent last.
			r.cache = append(append(r.cache[:i:i], r.cache[i+1:]...), c)
			return c, nil
		}
	}

	if err := r.cc.wait(ctx, r.name); err != nil {
		if r.cc.loadErr != nil {
			r.cc.kill()
			r.cc = spawnWorker(r.ccURL)
		}
		return compiled{}, err
	}

	msg := js.Global().Get("Object").New()
	msg.Set("code", code)
	if grader != "" {
		msg.Set("grader", grader)
	}
	r.seq++
	data, err := r.cc.post(ctx, r.seq, msg, time.After(compileTimeout))
	if err == errTimeout {
		// Probably stuck: restart the compiler.
		r.cc.kill()
		r.cc = spawnWorker(r.ccURL)
		return compiled{}, errCompileTimeout
	}
	if err != nil {
		return compiled{}, err
	}

	c := compiled{code: code, grader: grader, module: js.Undefined()}
	if data.Get("status").String() == "ok" {
		c.module = data.Get("module")
	} else {
		c.ce = data.Get("stderr").String()
	}
	if len(r.cache) == compileCache {
		r.cache = r.cache[1:]
	}
	r.cache = append(r.cache, c)
	return c, nil
}

func (r *CompileRunner) Run(ctx context.Context, code string, in judge.Input,
	limit time.Duration) (judge.RunResult, error) {

	c, err := r.compile(ctx, code, in.Grader)
	if err != nil {
		return judge.RunResult{}, err
	}
	module := c.module
	if module.IsUndefined() {
		return judge.RunResult{Status: judge.RunCompileError, Stderr: c.ce}, nil
	}

	msg := js.Global().Get("Object").New()
	msg.Set("module", module)
	setInput(msg, judge.Input{Stdin: in.Stdin, Interactor: in.Interactor}) // grader is linked in
	return r.exec.run(ctx, msg, limit)
}

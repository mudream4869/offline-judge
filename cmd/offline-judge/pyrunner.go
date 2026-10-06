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

const (
	loadTimeout = 2 * time.Minute
	// Wall-clock slack on top of the time limit before the worker is killed.
	killSlack = time.Second
)

// pyWorker is one Pyodide instance in a nested module worker.
type pyWorker struct {
	w       js.Value
	onMsg   js.Func
	ready   chan struct{} // closed when Pyodide has loaded
	dead    chan struct{} // closed when loading failed
	loadErr error
	results chan js.Value
}

func spawnWorker(url string) *pyWorker {
	pw := &pyWorker{
		ready:   make(chan struct{}),
		dead:    make(chan struct{}),
		results: make(chan js.Value, 1),
	}

	opts := js.Global().Get("Object").New()
	opts.Set("type", "module")
	pw.w = js.Global().Get("Worker").New(url, opts)

	// Never block inside a js.FuncOf callback.
	pw.onMsg = js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := args[0].Get("data")
		switch data.Get("type").String() {
		case "ready":
			close(pw.ready)
		case "error":
			pw.loadErr = errors.New(data.Get("error").String())
			close(pw.dead)
		case "result":
			select {
			case pw.results <- data:
			default:
			}
		}
		return nil
	})
	pw.w.Call("addEventListener", "message", pw.onMsg)
	return pw
}

func (pw *pyWorker) kill() {
	pw.w.Call("terminate")
	pw.onMsg.Release()
}

// PyRunner implements judge.Runner. It keeps a warm spare so that killing a
// timed-out worker doesn't cost a full Pyodide reload.
type PyRunner struct {
	mu    sync.Mutex
	url   string
	cur   *pyWorker
	spare *pyWorker
	seq   int
}

func NewPyRunner(url string) *PyRunner {
	r := &PyRunner{url: url, cur: spawnWorker(url)}
	go func() {
		// Start the spare after the first load, so they don't compete.
		select {
		case <-r.cur.ready:
		case <-r.cur.dead:
		}
		r.mu.Lock()
		if r.spare == nil {
			r.spare = spawnWorker(r.url)
		}
		r.mu.Unlock()
	}()
	return r
}

// Ready reports whether a run can start right away.
func (r *PyRunner) Ready() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.cur.ready:
		return true
	default:
		return false
	}
}

// replace kills the current worker and promotes the spare.
func (r *PyRunner) replace() {
	r.cur.kill()
	if r.spare != nil {
		r.cur = r.spare
	} else {
		r.cur = spawnWorker(r.url)
	}
	r.spare = spawnWorker(r.url)
}

func (r *PyRunner) Run(ctx context.Context, code, stdin string,
	limit time.Duration) (judge.RunResult, error) {

	r.mu.Lock()
	defer r.mu.Unlock()

	w := r.cur
	select {
	case <-w.ready:
	case <-w.dead:
		// Let the next attempt start over.
		r.replace()
		return judge.RunResult{}, w.loadErr
	case <-time.After(loadTimeout):
		return judge.RunResult{}, errors.New("Python 環境載入逾時")
	case <-ctx.Done():
		return judge.RunResult{}, ctx.Err()
	}

	r.seq++
	msg := js.Global().Get("Object").New()
	msg.Set("id", r.seq)
	msg.Set("code", code)
	msg.Set("stdin", stdin)
	w.w.Call("postMessage", msg)

	kill := time.NewTimer(limit + killSlack)
	defer kill.Stop()

	select {
	case data := <-w.results:
		res := judge.RunResult{
			Status: judge.RunStatus(data.Get("status").String()),
			Stdout: data.Get("stdout").String(),
			Stderr: data.Get("stderr").String(),
			Time:   time.Duration(data.Get("ms").Float() * float64(time.Millisecond)),
		}
		if data.Get("fatal").Bool() {
			r.replace()
		}
		return res, nil
	case <-kill.C:
		r.replace()
		return judge.RunResult{Status: judge.RunTimeout, Time: limit}, nil
	case <-ctx.Done():
		r.replace()
		return judge.RunResult{}, ctx.Err()
	}
}

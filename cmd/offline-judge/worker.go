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

// worker is a nested module worker speaking {type: ready|error|result}.
type worker struct {
	w       js.Value
	onMsg   js.Func
	ready   chan struct{} // closed when loaded
	dead    chan struct{} // closed when loading failed
	loadErr error
	results chan js.Value
}

func spawnWorker(url string) *worker {
	wk := &worker{
		ready:   make(chan struct{}),
		dead:    make(chan struct{}),
		results: make(chan js.Value, 8),
	}

	opts := js.Global().Get("Object").New()
	opts.Set("type", "module")
	wk.w = js.Global().Get("Worker").New(url, opts)

	// Never block inside a js.FuncOf callback.
	wk.onMsg = js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := args[0].Get("data")
		switch data.Get("type").String() {
		case "ready":
			close(wk.ready)
		case "error":
			wk.loadErr = errors.New(data.Get("error").String())
			close(wk.dead)
		case "result":
			select {
			case wk.results <- data:
			default:
			}
		}
		return nil
	})
	wk.w.Call("addEventListener", "message", wk.onMsg)
	return wk
}

// await blocks until wk has loaded or failed to.
func (wk *worker) await() {
	select {
	case <-wk.ready:
	case <-wk.dead:
	}
}

func (wk *worker) isReady() bool {
	select {
	case <-wk.ready:
		return true
	default:
		return false
	}
}

// wait blocks until the worker has loaded.
func (wk *worker) wait(ctx context.Context, name string) error {
	select {
	case <-wk.ready:
		return nil
	case <-wk.dead:
		return wk.loadErr
	case <-time.After(loadTimeout):
		return errors.New(name + " 環境載入逾時")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// post sends msg and returns the result with the same id, dropping stale
// results of cancelled requests.
func (wk *worker) post(ctx context.Context, id int, msg js.Value,
	timeout <-chan time.Time) (js.Value, error) {

	msg.Set("id", id)
	wk.w.Call("postMessage", msg)
	for {
		select {
		case data := <-wk.results:
			if data.Get("id").Int() == id {
				return data, nil
			}
		case <-timeout:
			return js.Value{}, errTimeout
		case <-ctx.Done():
			return js.Value{}, ctx.Err()
		}
	}
}

var errTimeout = errors.New("timeout")

func (wk *worker) kill() {
	wk.w.Call("terminate")
	wk.onMsg.Release()
}

// pool runs programs on one worker at a time and kills it when a run goes
// over its limit. A warm spare makes the kill cheap.
type pool struct {
	mu    sync.Mutex
	url   string
	name  string
	cur   *worker
	spare *worker
	seq   int
}

func newPool(url, name string) *pool {
	p := &pool{url: url, name: name, cur: spawnWorker(url)}
	go func() {
		// Start the spare after the first load, so they don't compete.
		p.cur.await()
		p.mu.Lock()
		if p.spare == nil {
			p.spare = spawnWorker(p.url)
		}
		p.mu.Unlock()
	}()
	return p
}

// Ready reports whether a run can start right away.
func (p *pool) Ready() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cur.isReady()
}

func (p *pool) awaitLoad() {
	p.mu.Lock()
	w := p.cur
	p.mu.Unlock()
	w.await()
}

// replace kills the current worker and promotes the spare.
func (p *pool) replace() {
	p.cur.kill()
	if p.spare != nil {
		p.cur = p.spare
	} else {
		p.cur = spawnWorker(p.url)
	}
	p.spare = spawnWorker(p.url)
}

// run posts msg (without id) and waits for a result shaped like
// {status, stdout, stderr, ms, fatal, judged?, iaError?}.
func (p *pool) run(ctx context.Context, msg js.Value,
	limit time.Duration) (judge.RunResult, error) {

	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := p.call(ctx, msg, limit+killSlack)
	switch {
	case err == errTimeout:
		return judge.RunResult{Status: judge.RunTimeout, Time: limit}, nil
	case err != nil:
		return judge.RunResult{}, err
	}

	res := judge.RunResult{
		Status: judge.RunStatus(data.Get("status").String()),
		Stdout: data.Get("stdout").String(),
		Stderr: data.Get("stderr").String(),
		Time:   time.Duration(data.Get("ms").Float() * float64(time.Millisecond)),
	}
	if data.Get("fatal").Bool() {
		p.replace()
	}
	if e := data.Get("iaError"); e.Truthy() {
		return judge.RunResult{}, errors.New("互動程式錯誤：" + e.String())
	}
	if j := data.Get("judged"); j.Truthy() {
		res.Judged = &judge.Judgement{OK: j.Get("ok").Bool(), Message: j.Get("message").String()}
	}
	return res, nil
}

// call posts msg (without id) and returns the result, killing the worker
// after timeout (errTimeout) or on cancel. p.mu must be held.
func (p *pool) call(ctx context.Context, msg js.Value, timeout time.Duration) (js.Value, error) {
	w := p.cur
	if err := w.wait(ctx, p.name); err != nil {
		if w.loadErr != nil {
			// Let the next attempt start over.
			p.replace()
		}
		return js.Value{}, err
	}

	kill := time.NewTimer(timeout)
	defer kill.Stop()

	p.seq++
	data, err := w.post(ctx, p.seq, msg, kill.C)
	if err != nil {
		p.replace()
	}
	return data, err
}

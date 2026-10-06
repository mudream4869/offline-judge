//go:build js && wasm

package main

import (
	"context"
	"syscall/js"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// PyRunner implements judge.Runner with Pyodide workers.
type PyRunner struct{ *pool }

func NewPyRunner(url string) *PyRunner {
	return &PyRunner{newPool(url, "Python")}
}

func (r *PyRunner) Run(ctx context.Context, code, stdin string,
	limit time.Duration) (judge.RunResult, error) {

	msg := js.Global().Get("Object").New()
	msg.Set("code", code)
	msg.Set("stdin", stdin)
	return r.run(ctx, msg, limit)
}

//go:build js && wasm

package main

import (
	"context"
	"syscall/js"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// CodeRunner implements judge.Runner with workers that take source code
// directly (pyworker.mjs, jsrun.mjs).
type CodeRunner struct{ *pool }

func NewCodeRunner(url, name string) *CodeRunner {
	return &CodeRunner{newPool(url, name)}
}

func (r *CodeRunner) Run(ctx context.Context, code, stdin string,
	limit time.Duration) (judge.RunResult, error) {

	msg := js.Global().Get("Object").New()
	msg.Set("code", code)
	msg.Set("stdin", stdin)
	return r.run(ctx, msg, limit)
}

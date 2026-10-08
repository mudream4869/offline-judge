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

func (r *CodeRunner) Run(ctx context.Context, code string, in judge.Input,
	limit time.Duration) (judge.RunResult, error) {

	msg := js.Global().Get("Object").New()
	msg.Set("code", code)
	setInput(msg, in)
	return r.run(ctx, msg, limit)
}

// setInput sets the stdin (and interactor) of a worker message.
func setInput(msg js.Value, in judge.Input) {
	msg.Set("stdin", in.Stdin)
	if in.Interactor != "" {
		msg.Set("interactor", in.Interactor)
	}
}

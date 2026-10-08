//go:build js && wasm

package main

import (
	"context"
	"errors"
	"syscall/js"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// checkTimeout bounds one call of a problem's checker.
const checkTimeout = 5 * time.Second

// CheckRunner runs checker.js in a killable worker (checker.mjs).
type CheckRunner struct{ *pool }

func NewCheckRunner(url string) *CheckRunner {
	return &CheckRunner{newPool(url, "checker")}
}

// Checker returns a judge.Checker for the checker source src.
func (r *CheckRunner) Checker(src string) judge.Checker {
	return func(ctx context.Context, c judge.Case, output string) (bool, string, error) {
		msg := js.Global().Get("Object").New()
		msg.Set("checker", src)
		msg.Set("input", c.Input)
		msg.Set("output", output)
		msg.Set("answer", c.Output)

		r.mu.Lock()
		defer r.mu.Unlock()
		data, err := r.call(ctx, msg, checkTimeout)
		switch {
		case err == errTimeout:
			return false, "", errors.New("checker 逾時")
		case err != nil:
			return false, "", err
		case data.Get("error").Truthy():
			return false, "", errors.New("checker 錯誤：" + data.Get("error").String())
		}
		return data.Get("ok").Bool(), data.Get("message").String(), nil
	}
}

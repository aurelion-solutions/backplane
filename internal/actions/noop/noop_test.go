// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
)

func newReg(t *testing.T) *registry.Registry {
	t.Helper()
	r := registry.New()
	Register(r)
	return r
}

func ctx() registry.ActionContext {
	return registry.ActionContext{Ctx: context.Background()}
}

func TestEcho(t *testing.T) {
	r := newReg(t)
	out, err := r.Dispatch("noop", "echo", map[string]any{"message": "hi"}, ctx())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if out["message"] != "hi" {
		t.Fatalf("message = %v", out["message"])
	}
}

func TestSleep_Positive(t *testing.T) {
	r := newReg(t)
	start := time.Now()
	out, err := r.Dispatch("noop", "sleep", map[string]any{"sleep_millis": 50}, ctx())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if out["slept_millis"].(float64) != 50 {
		t.Fatalf("slept_millis = %v", out["slept_millis"])
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatalf("sleep returned too soon: %v", time.Since(start))
	}
}

func TestSleep_RejectsNegative(t *testing.T) {
	r := newReg(t)
	_, err := r.Dispatch("noop", "sleep", map[string]any{"sleep_millis": -1}, ctx())
	if err == nil || !strings.Contains(err.Error(), "must be > 0") {
		t.Fatalf("want positive-validation error, got %v", err)
	}
}

func TestSleep_CancelPropagates(t *testing.T) {
	r := newReg(t)
	cctx, cancel := context.WithCancel(context.Background())
	actx := registry.ActionContext{Ctx: cctx}
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := r.Dispatch("noop", "sleep", map[string]any{"sleep_millis": 5000}, actx)
	if err == nil {
		t.Fatalf("want cancel error")
	}
}

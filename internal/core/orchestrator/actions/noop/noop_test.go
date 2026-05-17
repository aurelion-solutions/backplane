// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/core/events"
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

func TestFail_Raises(t *testing.T) {
	r := newReg(t)
	_, err := r.Dispatch("noop", "fail", map[string]any{"message": "boom"}, ctx())
	if err == nil {
		t.Fatalf("want error")
	}
	if !errors.Is(err, ErrDeliberate) {
		t.Fatalf("err = %v, want wraps ErrDeliberate", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want message embedded", err)
	}
}

func TestFail_RejectsEmptyMessage(t *testing.T) {
	r := newReg(t)
	_, err := r.Dispatch("noop", "fail", map[string]any{"message": ""}, ctx())
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("want empty-message validation, got %v", err)
	}
}

func TestConstant_RoundTrips(t *testing.T) {
	r := newReg(t)
	out, err := r.Dispatch("noop", "constant", map[string]any{
		"value": map[string]any{"k": "v", "n": 3.0},
	}, ctx())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	v, ok := out["value"].(map[string]any)
	if !ok {
		t.Fatalf("value type = %T", out["value"])
	}
	if v["k"] != "v" || v["n"] != 3.0 {
		t.Fatalf("value = %#v", v)
	}
}

func TestConstant_NilValueBecomesEmpty(t *testing.T) {
	r := newReg(t)
	out, err := r.Dispatch("noop", "constant", map[string]any{}, ctx())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	v, ok := out["value"].(map[string]any)
	if !ok || len(v) != 0 {
		t.Fatalf("want empty map, got %#v (type %T)", out["value"], out["value"])
	}
}

// fakeSink is a Sink implementation that records every envelope.
type fakeSink struct {
	mu   sync.Mutex
	envs []events.Envelope
	err  error
}

func (f *fakeSink) Emit(_ context.Context, e events.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.envs = append(f.envs, e)
	return nil
}

func emitCtx(sink events.Sink, runID uuid.UUID) registry.ActionContext {
	return registry.ActionContext{
		Ctx:           context.Background(),
		Events:        sink,
		PipelineRunID: runID,
	}
}

func TestEmit_PublishesEnvelope(t *testing.T) {
	r := newReg(t)
	sink := &fakeSink{}
	runID := uuid.New()
	out, err := r.Dispatch("noop", "emit", map[string]any{
		"event_type": "smoke.noop.fired",
		"payload":    map[string]any{"k": "v"},
	}, emitCtx(sink, runID))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(sink.envs) != 1 {
		t.Fatalf("envs = %d", len(sink.envs))
	}
	e := sink.envs[0]
	if e.EventType != "smoke.noop.fired" {
		t.Fatalf("event_type = %q", e.EventType)
	}
	if e.CorrelationID != runID.String() {
		t.Fatalf("correlation_id = %q, want fallback to run ID %q", e.CorrelationID, runID.String())
	}
	if e.Payload["k"] != "v" {
		t.Fatalf("payload = %#v", e.Payload)
	}
	if out["event_id"] != e.EventID.String() {
		t.Fatalf("result event_id mismatch: %v vs %v", out["event_id"], e.EventID)
	}
}

func TestEmit_HonoursExplicitCorrelationID(t *testing.T) {
	r := newReg(t)
	sink := &fakeSink{}
	_, err := r.Dispatch("noop", "emit", map[string]any{
		"event_type":     "smoke.noop.fired",
		"correlation_id": "corr-explicit",
	}, emitCtx(sink, uuid.New()))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if sink.envs[0].CorrelationID != "corr-explicit" {
		t.Fatalf("correlation_id = %q", sink.envs[0].CorrelationID)
	}
}

func TestEmit_RejectsMissingSink(t *testing.T) {
	r := newReg(t)
	_, err := r.Dispatch("noop", "emit", map[string]any{
		"event_type": "smoke.noop.fired",
	}, ctx())
	if err == nil || !strings.Contains(err.Error(), "events sink not wired") {
		t.Fatalf("want missing-sink error, got %v", err)
	}
}

func TestEmit_RejectsInvalidEventType(t *testing.T) {
	r := newReg(t)
	sink := &fakeSink{}
	_, err := r.Dispatch("noop", "emit", map[string]any{
		"event_type": "",
	}, emitCtx(sink, uuid.New()))
	if err == nil {
		t.Fatalf("want envelope validation error")
	}
}

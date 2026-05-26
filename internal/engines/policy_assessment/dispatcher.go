// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrUnknownMechanism is returned by Dispatcher.Evaluate when the
// request references a mechanism with no registered handler.
var ErrUnknownMechanism = errors.New("policy_assessment: unknown mechanism")

// Handler is the contract every mechanism implements. One handler per
// PolicyMechanism value; the dispatcher looks up by Request.Mechanism
// and delegates verbatim.
type Handler interface {
	// Mechanism is the registry key — "cedar", "sod", "llm_classification", …
	Mechanism() string

	// Prepare is invoked once per policy entry when the store
	// publishes a new snapshot. Handlers use it to load /
	// compile / cache anything expensive — Cedar policy parse,
	// LLM prompt template compile, SoD rule fetch. Idempotent.
	//
	// A failure on Prepare excludes the entry from future Evaluate
	// dispatches for this snapshot; the dispatcher logs and skips,
	// keeping the rest of the catalogue live.
	Prepare(ctx context.Context, entry Entry) error

	// Evaluate runs the mechanism against the supplied request.
	// The handler is goroutine-safe — many Evaluate calls may be
	// in flight simultaneously. Slow / blocking work belongs in
	// Prepare, not here.
	Evaluate(ctx context.Context, req Request) (Output, error)
}

// Dispatcher routes Requests to the handler registered for their
// Mechanism. Stateless from the caller's perspective; handlers carry
// any per-policy state internally.
//
// The registry is fixed at composition time — Register before serving
// traffic, do not mutate during evaluation.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewDispatcher returns an empty Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: map[string]Handler{}}
}

// Register adds a handler under handler.Mechanism(). Overwrites any
// prior registration.
func (d *Dispatcher) Register(h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[h.Mechanism()] = h
}

// Has reports whether a handler is registered for the named mechanism.
func (d *Dispatcher) Has(mechanism string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.handlers[mechanism]
	return ok
}

// Mechanisms returns the sorted set of mechanism names with a
// registered handler.
func (d *Dispatcher) Mechanisms() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]string, 0, len(d.handlers))
	for m := range d.handlers {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// PrepareAll runs Handler.Prepare on every entry whose mechanism is
// registered. Returns the count of entries successfully prepared
// (entries whose handler is not registered are silently skipped;
// preparation failures are returned as part of the error slice but
// do not abort the run).
func (d *Dispatcher) PrepareAll(ctx context.Context, entries []Entry) (int, []error) {
	var (
		ok   int
		errs []error
	)
	for _, e := range entries {
		d.mu.RLock()
		h, has := d.handlers[e.Manifest.Mechanism]
		d.mu.RUnlock()
		if !has {
			continue
		}
		if err := h.Prepare(ctx, e); err != nil {
			errs = append(errs, fmt.Errorf("prepare %s/%s: %w",
				e.CartridgeRef, e.Manifest.RuleID, err))
			continue
		}
		ok++
	}
	return ok, errs
}

// Evaluate dispatches one request to the registered handler. Returns
// ErrUnknownMechanism wrapped with the mechanism name when no handler
// is registered.
func (d *Dispatcher) Evaluate(ctx context.Context, req Request) (Output, error) {
	d.mu.RLock()
	h, ok := d.handlers[req.Mechanism]
	d.mu.RUnlock()
	if !ok {
		return Output{}, fmt.Errorf("%w: %q", ErrUnknownMechanism, req.Mechanism)
	}
	return h.Evaluate(ctx, req)
}

// EvaluateEntry is a convenience that builds a Request from an Entry
// and the supplied Facts, then dispatches.
//
// stack_check gate: when the entry declares a stack_check precondition,
// EvaluateEntry first checks every required truth-input key against
// facts.EvidencePresent. If any is absent, it short-circuits to a
// not_evaluable Output without running the mechanism — the rule cannot
// be judged on incomplete evidence (the Blind Spots path).
func (d *Dispatcher) EvaluateEntry(ctx context.Context, entry Entry, facts Facts) (Output, error) {
	if missing := missingEvidence(entry, facts); len(missing) > 0 {
		return Output{NotEvaluable: true, MissingEvidence: missing}, nil
	}
	return d.Evaluate(ctx, Request{
		Mechanism:    entry.Manifest.Mechanism,
		PolicyID:     entry.CartridgeRef + "/" + entry.Manifest.RuleID,
		CartridgeRef: entry.CartridgeRef,
		BasePath:     entry.Manifest.BasePath,
		Body:         entry.Manifest.Body,
		Facts:        facts,
	})
}

// missingEvidence returns the stack_check required keys the facts do
// not carry evidence for, in declaration order. Empty when there is no
// stack_check or all required evidence is present.
func missingEvidence(entry Entry, facts Facts) []string {
	if entry.Manifest.StackCheck == nil {
		return nil
	}
	var missing []string
	for _, key := range entry.Manifest.StackCheck.Requires {
		if !facts.EvidencePresent[key] {
			missing = append(missing, key)
		}
	}
	return missing
}

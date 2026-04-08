// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package registry

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type fooArgs struct {
	Subject string `json:"subject"`
}

type fooResult struct {
	Greeting string `json:"greeting"`
}

func fooHandler(args fooArgs, _ ActionContext) (fooResult, error) {
	if args.Subject == "boom" {
		return fooResult{}, errors.New("boom")
	}
	return fooResult{Greeting: "hello " + args.Subject}, nil
}

func newCtx() ActionContext {
	return ActionContext{Ctx: context.Background()}
}

func TestRegister_And_Dispatch(t *testing.T) {
	r := New()
	if err := Register[fooArgs, fooResult](r, "foo", "greet", true, fooHandler); err != nil {
		t.Fatalf("Register: %v", err)
	}

	out, err := r.Dispatch("foo", "greet", map[string]any{"subject": "world"}, newCtx())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if out["greeting"] != "hello world" {
		t.Fatalf("greeting = %v, want %q", out["greeting"], "hello world")
	}
}

func TestRegister_Duplicate(t *testing.T) {
	r := New()
	_ = Register[fooArgs, fooResult](r, "foo", "greet", true, fooHandler)
	err := Register[fooArgs, fooResult](r, "foo", "greet", true, fooHandler)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestDispatch_NotFound(t *testing.T) {
	r := New()
	_, err := r.Dispatch("foo", "missing", nil, newCtx())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDispatch_ArgsValidation(t *testing.T) {
	r := New()
	_ = Register[fooArgs, fooResult](r, "foo", "greet", true, fooHandler)
	// Subject must be string; passing an int fails JSON Schema.
	_, err := r.Dispatch("foo", "greet", map[string]any{"subject": 42}, newCtx())
	if !errors.Is(err, ErrArgsValidation) {
		t.Fatalf("want ErrArgsValidation, got %v", err)
	}
}

func TestDispatch_HandlerError(t *testing.T) {
	r := New()
	_ = Register[fooArgs, fooResult](r, "foo", "greet", true, fooHandler)
	_, err := r.Dispatch("foo", "greet", map[string]any{"subject": "boom"}, newCtx())
	if err == nil || err.Error() != "boom" {
		t.Fatalf("want handler error, got %v", err)
	}
}

func TestHas_LoaderShim(t *testing.T) {
	r := New()
	_ = Register[fooArgs, fooResult](r, "foo", "greet", true, fooHandler)
	if !r.Has("foo", "greet") {
		t.Fatalf("Has(foo,greet) = false")
	}
	if r.Has("foo", "missing") {
		t.Fatalf("Has(foo,missing) = true")
	}
}

func TestAll_SortedDeterministic(t *testing.T) {
	r := New()
	for _, pair := range [][2]string{{"zeta", "y"}, {"alpha", "x"}, {"alpha", "a"}} {
		eng, act := pair[0], pair[1]
		// fresh inline handler so duplicate pairs would still register
		// each unique combination.
		_ = Register[fooArgs, fooResult](r, eng, act, true, fooHandler)
	}
	got := r.All()
	want := [][2]string{{"alpha", "a"}, {"alpha", "x"}, {"zeta", "y"}}
	for i, e := range got {
		if e.Engine != want[i][0] || e.Action != want[i][1] {
			t.Fatalf("All[%d] = (%q, %q), want %v", i, e.Engine, e.Action, want[i])
		}
	}
}

func TestRegister_RejectsEmptyKey(t *testing.T) {
	r := New()
	if err := Register[fooArgs, fooResult](r, "", "x", true, fooHandler); err == nil {
		t.Fatalf("want error on empty engine")
	}
	if err := Register[fooArgs, fooResult](r, "x", "", true, fooHandler); err == nil {
		t.Fatalf("want error on empty action")
	}
}

// Avoid unused-fmt warnings if local helpers shift.
var _ = fmt.Sprintf

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package correlation

import (
	"context"
	"testing"
)

func TestWithID_Empty_NoOp(t *testing.T) {
	ctx := context.Background()
	out := WithID(ctx, "")
	if _, ok := ID(out); ok {
		t.Fatalf("empty id must not be stored")
	}
}

func TestWithID_RoundTrip(t *testing.T) {
	ctx := WithID(context.Background(), "abc-123")
	got, ok := ID(ctx)
	if !ok || got != "abc-123" {
		t.Fatalf("want (abc-123, true), got (%q, %v)", got, ok)
	}
}

func TestID_NoValue(t *testing.T) {
	if _, ok := ID(context.Background()); ok {
		t.Fatalf("expected absence")
	}
}

func TestEnsure_ReusesExisting(t *testing.T) {
	ctx := WithID(context.Background(), "preset")
	_, id := Ensure(ctx)
	if id != "preset" {
		t.Fatalf("Ensure must not replace existing id, got %q", id)
	}
}

func TestEnsure_GeneratesWhenMissing(t *testing.T) {
	ctx, id := Ensure(context.Background())
	if id == "" {
		t.Fatalf("Ensure must generate a non-empty id")
	}
	got, ok := ID(ctx)
	if !ok || got != id {
		t.Fatalf("Ensure must attach the id to ctx")
	}
}

func TestID_NilCtx(t *testing.T) {
	if _, ok := ID(nil); ok {
		t.Fatalf("nil ctx must not panic and must report absence")
	}
}

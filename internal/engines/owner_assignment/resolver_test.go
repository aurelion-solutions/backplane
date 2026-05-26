// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package owner_assignment

import (
	"testing"

	"github.com/google/uuid"
)

func TestForApplication(t *testing.T) {
	app := uuid.New()
	other := uuid.New()
	r := NewResolver(map[uuid.UUID]string{app: "platform-team@corp.example"})

	if got := r.ForApplication(app); got != "platform-team@corp.example" {
		t.Fatalf("expected owner, got %q", got)
	}
	if got := r.ForApplication(other); got != "" {
		t.Fatalf("unknown app should resolve to empty owner, got %q", got)
	}
}

func TestForApplication_NilResolver(t *testing.T) {
	var r *Resolver
	if got := r.ForApplication(uuid.New()); got != "" {
		t.Fatalf("nil resolver should answer empty, got %q", got)
	}
}

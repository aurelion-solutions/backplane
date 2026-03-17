// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"testing"

	"github.com/google/uuid"
)

func inst(id string, tags ...string) *ConnectorInstance {
	return &ConnectorInstance{ID: uuid.New(), InstanceID: id, Tags: tags}
}

func TestMatching_EmptyRequired_MatchesAll(t *testing.T) {
	pool := []*ConnectorInstance{inst("a"), inst("b", "x"), inst("c", "y", "z")}
	got := Matching(pool, nil)
	if len(got) != 3 {
		t.Fatalf("want 3 matches, got %d", len(got))
	}
}

func TestMatching_RequiresSuperset(t *testing.T) {
	pool := []*ConnectorInstance{
		inst("a", "salesforce"),
		inst("b", "salesforce", "prod"),
		inst("c", "salesforce", "prod", "eu"),
		inst("d", "github"),
	}
	got := Matching(pool, []string{"salesforce", "prod"})
	if len(got) != 2 {
		t.Fatalf("want 2 matches, got %d", len(got))
	}
	wantIDs := map[string]bool{"b": true, "c": true}
	for _, g := range got {
		if !wantIDs[g.InstanceID] {
			t.Fatalf("unexpected match %q", g.InstanceID)
		}
	}
}

func TestPick_TakesFirstMatch(t *testing.T) {
	pool := []*ConnectorInstance{
		inst("a", "github"),
		inst("b", "salesforce", "prod"),
		inst("c", "salesforce", "prod", "eu"),
	}
	got := Pick(pool, []string{"salesforce", "prod"})
	if got == nil {
		t.Fatalf("expected a pick")
	}
	if got.InstanceID != "b" {
		t.Fatalf("want b (first match), got %q", got.InstanceID)
	}
}

func TestPick_NoMatch_ReturnsNil(t *testing.T) {
	pool := []*ConnectorInstance{inst("a", "github")}
	if got := Pick(pool, []string{"salesforce"}); got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func TestPick_EmptyPool_ReturnsNil(t *testing.T) {
	if got := Pick(nil, []string{"any"}); got != nil {
		t.Fatalf("want nil for empty pool")
	}
}

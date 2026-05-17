// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"testing"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/inventory/initiatives"
)

func makeCurrent(appID uuid.UUID, capID *uuid.UUID, kind, sourceRuleID string) *initiatives.Initiative {
	just := map[string]any{}
	if sourceRuleID != "" {
		just["source_rule_id"] = sourceRuleID
	}
	return &initiatives.Initiative{
		ID:            uuid.New(),
		Kind:          kind,
		ApplicationID: appID,
		CapabilityID:  capID,
		Justification: just,
	}
}

func TestDiff_BothEmpty(t *testing.T) {
	toCreate, toTombstone := diff(nil, nil)
	if len(toCreate) != 0 || len(toTombstone) != 0 {
		t.Fatalf("empty inputs → empty outputs, got %d/%d", len(toCreate), len(toTombstone))
	}
}

func TestDiff_AllPlannedNew(t *testing.T) {
	appID := uuid.New()
	planned := []plannedInitiative{
		{Kind: "inheritance", ApplicationID: appID, SourceRuleID: "rule-a"},
	}
	toCreate, toTombstone := diff(planned, nil)
	if len(toCreate) != 1 || toCreate[0].SourceRuleID != "rule-a" {
		t.Fatalf("expected one create for rule-a, got %+v", toCreate)
	}
	if len(toTombstone) != 0 {
		t.Fatalf("nothing to tombstone, got %d", len(toTombstone))
	}
}

func TestDiff_AllCurrentRemoved(t *testing.T) {
	appID := uuid.New()
	current := []*initiatives.Initiative{
		makeCurrent(appID, nil, "inheritance", "rule-a"),
	}
	toCreate, toTombstone := diff(nil, current)
	if len(toCreate) != 0 {
		t.Fatalf("nothing to create, got %d", len(toCreate))
	}
	if len(toTombstone) != 1 {
		t.Fatalf("expected one tombstone, got %d", len(toTombstone))
	}
}

func TestDiff_MatchIsNoOp(t *testing.T) {
	appID := uuid.New()
	capID := uuid.New()
	planned := []plannedInitiative{
		{Kind: "inheritance", ApplicationID: appID, CapabilityID: &capID, SourceRuleID: "rule-a"},
	}
	current := []*initiatives.Initiative{
		makeCurrent(appID, &capID, "inheritance", "rule-a"),
	}
	toCreate, toTombstone := diff(planned, current)
	if len(toCreate) != 0 || len(toTombstone) != 0 {
		t.Fatalf("match should be no-op, got %d/%d", len(toCreate), len(toTombstone))
	}
}

func TestDiff_SameTargetDifferentRules(t *testing.T) {
	// Two different rules pointing at the same (kind, app, cap) are
	// two distinct logical initiatives. Neither should collapse into
	// a duplicate.
	appID := uuid.New()
	planned := []plannedInitiative{
		{Kind: "inheritance", ApplicationID: appID, SourceRuleID: "rule-a"},
		{Kind: "inheritance", ApplicationID: appID, SourceRuleID: "rule-b"},
	}
	toCreate, toTombstone := diff(planned, nil)
	if len(toCreate) != 2 {
		t.Fatalf("two rules → two creates, got %d", len(toCreate))
	}
	if len(toTombstone) != 0 {
		t.Fatalf("nothing to tombstone, got %d", len(toTombstone))
	}
}

func TestDiff_AccountInitVsGrantInitAreDistinct(t *testing.T) {
	// CapabilityID nil (account-init) and CapabilityID set
	// (grant-init) are different keys even when kind / app / rule
	// match — they live on different rows in the schema.
	appID := uuid.New()
	capID := uuid.New()
	planned := []plannedInitiative{
		{Kind: "inheritance", ApplicationID: appID, SourceRuleID: "rule-a"},
		{Kind: "inheritance", ApplicationID: appID, CapabilityID: &capID, SourceRuleID: "rule-a"},
	}
	toCreate, _ := diff(planned, nil)
	if len(toCreate) != 2 {
		t.Fatalf("account-init and grant-init are distinct, got %d", len(toCreate))
	}
}

func TestDiff_PartialOverlap(t *testing.T) {
	appID1, appID2, appID3 := uuid.New(), uuid.New(), uuid.New()
	planned := []plannedInitiative{
		{Kind: "inheritance", ApplicationID: appID1, SourceRuleID: "r1"}, // matches current[0] → no-op
		{Kind: "inheritance", ApplicationID: appID2, SourceRuleID: "r2"}, // not in current → create
	}
	current := []*initiatives.Initiative{
		makeCurrent(appID1, nil, "inheritance", "r1"), // matches planned[0]
		makeCurrent(appID3, nil, "inheritance", "r3"), // not in planned → tombstone
	}
	toCreate, toTombstone := diff(planned, current)
	if len(toCreate) != 1 || toCreate[0].SourceRuleID != "r2" {
		t.Fatalf("expected create r2, got %+v", toCreate)
	}
	if len(toTombstone) != 1 || toTombstone[0].ApplicationID != appID3 {
		t.Fatalf("expected tombstone appID3, got %+v", toTombstone)
	}
}

func TestDiff_CurrentWithoutSourceRuleIDIsTreatedAsRule(t *testing.T) {
	// A current row whose Justification has no source_rule_id matches
	// a planned with empty SourceRuleID — both map to the empty
	// string slot. This is the recovery path for legacy / manually-
	// inserted rows that predate the engine.
	appID := uuid.New()
	planned := []plannedInitiative{
		{Kind: "inheritance", ApplicationID: appID, SourceRuleID: ""},
	}
	current := []*initiatives.Initiative{
		makeCurrent(appID, nil, "inheritance", ""),
	}
	toCreate, toTombstone := diff(planned, current)
	if len(toCreate) != 0 || len(toTombstone) != 0 {
		t.Fatalf("empty source_rule_id should match, got %d/%d", len(toCreate), len(toTombstone))
	}
}

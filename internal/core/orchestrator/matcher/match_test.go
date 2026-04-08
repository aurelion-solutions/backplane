// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package matcher

import (
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
)

func TestPayloadSatisfiesMatch_EmptyMatch(t *testing.T) {
	if !PayloadSatisfiesMatch(map[string]any{}, map[string]any{"x": 1}) {
		t.Fatal("empty match should match any payload")
	}
}

func TestPayloadSatisfiesMatch_FlatEquality(t *testing.T) {
	m := map[string]any{"app": "jira"}
	if !PayloadSatisfiesMatch(m, map[string]any{"app": "jira", "extra": 1}) {
		t.Fatal("flat equality + payload superset should match")
	}
	if PayloadSatisfiesMatch(m, map[string]any{"app": "slack"}) {
		t.Fatal("scalar mismatch must reject")
	}
	if PayloadSatisfiesMatch(m, map[string]any{}) {
		t.Fatal("missing key in payload must reject")
	}
}

func TestPayloadSatisfiesMatch_Nested(t *testing.T) {
	m := map[string]any{"changes": map[string]any{"status": "active"}}
	pHit := map[string]any{"changes": map[string]any{"status": "active", "other": 1}, "x": 9}
	pMiss := map[string]any{"changes": map[string]any{"status": "terminated"}}
	if !PayloadSatisfiesMatch(m, pHit) {
		t.Fatal("nested map containment should match")
	}
	if PayloadSatisfiesMatch(m, pMiss) {
		t.Fatal("nested scalar mismatch must reject")
	}
}

func TestPayloadSatisfiesMatch_ListSetContainment(t *testing.T) {
	m := map[string]any{"tags": []any{"a", "b"}}
	if !PayloadSatisfiesMatch(m, map[string]any{"tags": []any{"a", "b", "c"}}) {
		t.Fatal("primitive list set-containment should match superset")
	}
	if PayloadSatisfiesMatch(m, map[string]any{"tags": []any{"a"}}) {
		t.Fatal("missing required element must reject")
	}
}

func TestFindMatchingMQTriggers(t *testing.T) {
	a := &loader.Definition{
		Name: "a",
		Triggers: []map[string]any{
			{"type": "mq", "routing_key": "inv.user.created"},
		},
	}
	b := &loader.Definition{
		Name: "b",
		Triggers: []map[string]any{
			{"type": "mq", "routing_key": "inv.user.created",
				"match": map[string]any{"tenant_id": "x"}},
		},
	}
	c := &loader.Definition{
		Name: "c",
		Triggers: []map[string]any{
			{"type": "schedule", "every": "1h"}, // ignored — wrong type
		},
	}

	defs := []*loader.Definition{a, b, c}
	hits := FindMatchingMQTriggers(defs, "inv.user.created",
		map[string]any{"tenant_id": "x", "id": "u1"})
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2 (a + b)", len(hits))
	}

	hits2 := FindMatchingMQTriggers(defs, "inv.user.created",
		map[string]any{"tenant_id": "y"})
	if len(hits2) != 1 || hits2[0].Definition != a {
		t.Fatalf("only a should match when tenant mismatches: %v", hits2)
	}
}

func TestExtractArgsFromPayload(t *testing.T) {
	spec := map[string]any{
		"subject_ref":    "subject_ref",
		"idempotency_key": "trace.id",
	}
	payload := map[string]any{
		"subject_ref": "00000000-0000-0000-0000-000000000001",
		"trace":       map[string]any{"id": "trace-42"},
	}
	got := ExtractArgsFromPayload(spec, payload)
	if got["subject_ref"] != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("subject_ref = %v", got["subject_ref"])
	}
	if got["idempotency_key"] != "trace-42" {
		t.Fatalf("idempotency_key = %v", got["idempotency_key"])
	}

	miss := ExtractArgsFromPayload(map[string]any{"x": "no.such.path"}, payload)
	if miss["x"] != nil {
		t.Fatalf("missing path should be nil, got %v", miss["x"])
	}
}

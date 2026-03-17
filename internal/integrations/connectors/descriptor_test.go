// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"encoding/json"
	"testing"
)

func TestCapabilityDescriptor_JSONRoundTrip(t *testing.T) {
	raw := `{
		"operations": [
			{
				"kind": "account_create",
				"dependency_rules": [
					{"resource": "license", "status": ["active"]}
				]
			}
		],
		"account_status": {
			"transitions": [["not_exists", "invited"], ["invited", "active"]]
		},
		"verify_fact_supported": true,
		"supported_fact_kinds": ["role_grant"],
		"cascades": {
			"before_disable": [{"fact_kind": "role"}]
		}
	}`
	var d CapabilityDescriptor
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(d.Operations) != 1 || d.Operations[0].Kind != "account_create" {
		t.Fatalf("unexpected operations: %+v", d.Operations)
	}
	if !d.VerifyFactSupported || len(d.SupportedFactKinds) != 1 {
		t.Fatalf("unexpected fact kinds")
	}
	if got := d.AccountStatus.Transitions; len(got) != 2 || got[0][0] != "not_exists" {
		t.Fatalf("transitions decoded wrong: %+v", got)
	}
	if len(d.Cascades.BeforeDisable) != 1 || d.Cascades.BeforeDisable[0].FactKind != "role" {
		t.Fatalf("cascades decoded wrong: %+v", d.Cascades.BeforeDisable)
	}

	// Re-marshal and ensure field names match the wire contract.
	encoded, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	for _, k := range []string{"operations", "account_status", "verify_fact_supported", "supported_fact_kinds", "cascades"} {
		if _, ok := generic[k]; !ok {
			t.Fatalf("re-encoded JSON must contain field %q (got %v)", k, generic)
		}
	}
}

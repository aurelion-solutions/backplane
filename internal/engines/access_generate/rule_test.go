// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"strings"
	"testing"
)

func TestParseInheritanceRule_Happy(t *testing.T) {
	body := map[string]any{
		"source_org_unit_dn": "corp/europe/engineering",
		"grants": []any{
			map[string]any{"application_slug": "microsoft_ad"},
			map[string]any{"application_slug": "github", "capability_slug": "developer"},
		},
	}
	rule, err := ParseInheritanceRule("popular.eng_dept", body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rule.RuleID != "popular.eng_dept" {
		t.Errorf("rule_id = %q, want popular.eng_dept", rule.RuleID)
	}
	if rule.SourceOrgUnitDN != "corp/europe/engineering" {
		t.Errorf("dn = %q", rule.SourceOrgUnitDN)
	}
	if len(rule.Grants) != 2 {
		t.Fatalf("grants count = %d, want 2", len(rule.Grants))
	}
	if rule.Grants[0].ApplicationSlug != "microsoft_ad" || rule.Grants[0].CapabilitySlug != "" {
		t.Errorf("grants[0] = %+v", rule.Grants[0])
	}
	if rule.Grants[1].ApplicationSlug != "github" || rule.Grants[1].CapabilitySlug != "developer" {
		t.Errorf("grants[1] = %+v", rule.Grants[1])
	}
}

func TestParseInheritanceRule_EmptyBody(t *testing.T) {
	if _, err := ParseInheritanceRule("rule", nil); err == nil {
		t.Fatalf("expected error on nil body")
	}
	if _, err := ParseInheritanceRule("rule", map[string]any{}); err == nil {
		t.Fatalf("expected error on empty map body")
	}
}

func TestParseInheritanceRule_MissingDN(t *testing.T) {
	body := map[string]any{
		"grants": []any{map[string]any{"application_slug": "x"}},
	}
	_, err := ParseInheritanceRule("rule", body)
	if err == nil || !strings.Contains(err.Error(), "source_org_unit_dn") {
		t.Fatalf("expected source_org_unit_dn error, got %v", err)
	}
}

func TestParseInheritanceRule_NoGrants(t *testing.T) {
	body := map[string]any{
		"source_org_unit_dn": "x",
		"grants":             []any{},
	}
	if _, err := ParseInheritanceRule("rule", body); err == nil {
		t.Fatalf("expected error on empty grants")
	}
}

func TestParseInheritanceRule_MissingApplicationSlug(t *testing.T) {
	body := map[string]any{
		"source_org_unit_dn": "x",
		"grants": []any{
			map[string]any{"capability_slug": "y"},
		},
	}
	_, err := ParseInheritanceRule("rule", body)
	if err == nil || !strings.Contains(err.Error(), "application_slug") {
		t.Fatalf("expected application_slug error, got %v", err)
	}
}

func TestParseInheritanceRule_MalformedTypes(t *testing.T) {
	// grants is the wrong shape — string instead of list.
	body := map[string]any{
		"source_org_unit_dn": "x",
		"grants":             "not a list",
	}
	if _, err := ParseInheritanceRule("rule", body); err == nil {
		t.Fatalf("expected parse error on bad grants type")
	}
}

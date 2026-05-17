// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"sort"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/inventory/initiatives"
)

// diff turns two sides — planned (what should exist) and current
// (what does exist right now, active) — into two action lists:
//
//   - toCreate: planned entries with no live match in current
//   - toTombstone: live current rows that no longer appear in planned
//
// Matching key is (kind, application_id, capability_id, source_rule_id).
// `source_rule_id` lives inside `Justification` for v1 — sources
// write it on Create, so on the current side we read it back from
// `Justification["source_rule_id"]`. Two planned entries from
// different rules pointing at the same (application, capability)
// produce two separate initiatives — that's exactly what the user
// asked for: multiple active initiatives per target are allowed.
//
// The function is pure: no I/O, no DB calls. Both sides are slices
// of small structs, and diff complexity is O(n + m) using a map
// lookup.
func diff(planned []plannedInitiative, current []*initiatives.Initiative) (toCreate []plannedInitiative, toTombstone []*initiatives.Initiative) {
	plannedByKey := make(map[matchKey]plannedInitiative, len(planned))
	for _, p := range planned {
		plannedByKey[keyOfPlanned(p)] = p
	}

	currentByKey := make(map[matchKey]*initiatives.Initiative, len(current))
	for _, c := range current {
		currentByKey[keyOfCurrent(c)] = c
	}

	for k, p := range plannedByKey {
		if _, hit := currentByKey[k]; !hit {
			toCreate = append(toCreate, p)
		}
	}
	for k, c := range currentByKey {
		if _, hit := plannedByKey[k]; !hit {
			toTombstone = append(toTombstone, c)
		}
	}
	return toCreate, toTombstone
}

// matchKey identifies one "logical" initiative regardless of its row
// id — same target + same source rule = same initiative.
type matchKey struct {
	kind          string
	applicationID uuid.UUID
	capabilityID  uuid.UUID // zero UUID = account-init (capability_id NULL)
	sourceRuleID  string
}

func keyOfPlanned(p plannedInitiative) matchKey {
	k := matchKey{
		kind:          p.Kind,
		applicationID: p.ApplicationID,
		sourceRuleID:  p.SourceRuleID,
	}
	if p.CapabilityID != nil {
		k.capabilityID = *p.CapabilityID
	}
	return k
}

func keyOfCurrent(c *initiatives.Initiative) matchKey {
	k := matchKey{
		kind:          c.Kind,
		applicationID: c.ApplicationID,
	}
	if c.CapabilityID != nil {
		k.capabilityID = *c.CapabilityID
	}
	if v, ok := c.Justification["source_rule_id"].(string); ok {
		k.sourceRuleID = v
	}
	return k
}

// sortRulesByID gives loadInheritanceRules a stable output order so
// diff results (and audit trails downstream) do not flip between
// runs based on cartridge file iteration order.
func sortRulesByID(rs []InheritanceRule) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].RuleID < rs[j].RuleID })
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/aurelion-solutions/backplane/internal/inventory/capabilities"
	"github.com/aurelion-solutions/backplane/internal/inventory/initiatives"
	"github.com/aurelion-solutions/backplane/internal/inventory/org_units"
	"github.com/aurelion-solutions/backplane/internal/inventory/principals"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
)

// orgUnitDNSep is the joiner used to render a leaf-to-root OrgUnit
// chain into a single distinguished name. Slash matches typical
// cartridge YAML convention ("corp/europe/engineering").
const orgUnitDNSep = "/"

// orgUnitDNMaxDepth bounds how far up the parent chain
// buildOrgUnitDN walks before it gives up. A safety cap against a
// pathological cycle in the org_units table — a real production tree
// is typically under 10 levels deep.
const orgUnitDNMaxDepth = 64

// plannedInitiative is the in-memory shape every source produces.
// Sources fan-in into a single []plannedInitiative which is then
// diffed against the current persisted active set.
//
// Justification is the source-specific payload that ends up verbatim
// in the `Initiative.Justification` jsonb column on Create. Each
// source MUST set `source_rule_id` inside Justification so the diff
// can match it against the current rows.
//
// SourceRuleID is also kept as a separate field so the engine can
// stamp it into `Initiative.Actor` (`<engine actor>:<rule id>`)
// without re-reading the jsonb on each Create.
type plannedInitiative struct {
	ApplicationID uuid.UUID
	CapabilityID  *uuid.UUID
	Kind          string
	Justification map[string]any
	SourceRuleID  string
}

// resolveInheritanceInitiatives projects the principal's structural
// attachment (Employment → OrgUnit) through inheritance rules loaded
// from the cartridge bundle.
//
// Inheritance is defined only for employment-backed principals.
// Workload and customer principals get nothing here — those bodies
// have their own future rule families.
//
// An employment whose validity window has closed contributes
// nothing: the recompute is "in force right now", not historical.
// When generative later runs for a principal that has gone
// employment-inactive, the planned set comes back empty and the
// diff tombstones every previously-issued inheritance initiative.
func (e *Engine) resolveInheritanceInitiatives(ctx context.Context, _ bun.IDB, principalID uuid.UUID, _ RecomputeFilter) ([]plannedInitiative, error) {
	rules, err := e.loadInheritanceRules(ctx)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, nil
	}

	principal, err := e.deps.Principals.GetByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principals.ErrNotFound) {
			return nil, fmt.Errorf("principal %s: %w", principalID, err)
		}
		return nil, err
	}
	if principal.Kind != shared.PrincipalKindEmployment {
		// Inheritance rules only project onto employment bodies for v1.
		return nil, nil
	}
	if principal.PrincipalEmploymentID == nil {
		// Data integrity: an employment-kind principal without a
		// body pointer is a bad row. Surface it instead of silently
		// returning empty.
		return nil, fmt.Errorf("principal %s: kind=employment but PrincipalEmploymentID is nil", principalID)
	}

	emp, err := e.deps.Employments.GetByID(ctx, *principal.PrincipalEmploymentID)
	if err != nil {
		return nil, fmt.Errorf("employment %s: %w", *principal.PrincipalEmploymentID, err)
	}
	if !emp.IsActiveAt(time.Now()) {
		return nil, nil
	}
	if emp.OrgUnitID == nil {
		// Employment without an OU contributes nothing — there is
		// no DN to match against rules.
		return nil, nil
	}

	dn, err := e.buildOrgUnitDN(ctx, *emp.OrgUnitID)
	if err != nil {
		return nil, fmt.Errorf("org_unit DN: %w", err)
	}

	planned := []plannedInitiative{}
	for i := range rules {
		rule := rules[i]
		if rule.SourceOrgUnitDN != dn {
			continue
		}
		for _, g := range rule.Grants {
			p, err := e.expandGrant(ctx, rule, g)
			if err != nil {
				return nil, err
			}
			planned = append(planned, p)
		}
	}
	return planned, nil
}

// expandGrant turns one RuleGrant + its owning rule into a single
// plannedInitiative with the slugs resolved to ids. Returned errors
// always identify the offending rule + slug — they end up in the
// engine's failure log, so operators can fix the cartridge.
func (e *Engine) expandGrant(ctx context.Context, rule InheritanceRule, g RuleGrant) (plannedInitiative, error) {
	app, err := e.deps.Applications.GetByCode(ctx, g.ApplicationSlug)
	if err != nil {
		return plannedInitiative{}, fmt.Errorf("rule %q: application_slug %q: %w", rule.RuleID, g.ApplicationSlug, err)
	}

	just := map[string]any{
		"source_rule_id":     rule.RuleID,
		"source_org_unit_dn": rule.SourceOrgUnitDN,
	}
	p := plannedInitiative{
		ApplicationID: app.ID,
		Kind:          initiatives.KindInheritance,
		Justification: just,
		SourceRuleID:  rule.RuleID,
	}

	if g.CapabilitySlug != "" {
		cap, err := e.deps.Capabilities.GetBySlug(ctx, g.CapabilitySlug)
		if err != nil {
			if errors.Is(err, capabilities.ErrNotFound) {
				return plannedInitiative{}, fmt.Errorf("rule %q: capability_slug %q: %w", rule.RuleID, g.CapabilitySlug, err)
			}
			return plannedInitiative{}, err
		}
		capID := cap.ID
		p.CapabilityID = &capID
		just["capability_slug"] = g.CapabilitySlug
	}
	return p, nil
}

// buildOrgUnitDN walks from the given leaf OrgUnit up the
// parent_id chain and joins the names with "/". Result is the
// distinguished name rules match against.
//
// The walk is bounded by orgUnitDNMaxDepth to keep a pathological
// cycle in the data from spinning forever; in practice a real tree
// is far shallower than the cap.
func (e *Engine) buildOrgUnitDN(ctx context.Context, leafID uuid.UUID) (string, error) {
	parts := make([]string, 0, 8)
	current := leafID
	for depth := 0; depth < orgUnitDNMaxDepth; depth++ {
		ou, err := e.deps.OrgUnits.GetByID(ctx, current)
		if err != nil {
			if errors.Is(err, org_units.ErrNotFound) {
				return "", fmt.Errorf("org_unit %s: %w", current, err)
			}
			return "", err
		}
		parts = append(parts, ou.Name)
		if ou.ParentID == nil {
			// Reached root — reverse and join.
			reverseInPlace(parts)
			return strings.Join(parts, orgUnitDNSep), nil
		}
		current = *ou.ParentID
	}
	return "", fmt.Errorf("org_unit DN walk exceeded depth %d starting at %s", orgUnitDNMaxDepth, leafID)
}

// reverseInPlace flips a string slice end-to-end. Used after the
// leaf-to-root walk so the final slice reads root-to-leaf.
func reverseInPlace(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// loadInheritanceRules pulls every policy manifest from the configured
// bundle, filters by mechanism, and parses each body into an
// InheritanceRule. Returns the parsed slice in stable rule-id order
// so downstream diffs do not depend on cartridge file iteration.
func (e *Engine) loadInheritanceRules(_ context.Context) ([]InheritanceRule, error) {
	manifests, err := e.deps.Cartridges.Policies(e.deps.BundleRef)
	if err != nil {
		return nil, err
	}
	out := make([]InheritanceRule, 0, len(manifests))
	for ruleID, m := range manifests {
		if m.Mechanism != MechanismInheritance {
			continue
		}
		r, err := ParseInheritanceRule(ruleID, m.Body)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	sortRulesByID(out)
	return out, nil
}

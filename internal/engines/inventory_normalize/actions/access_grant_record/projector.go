// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_grant_record

import (
	"path"
	"time"

	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/aurelion-solutions/backplane/internal/inventory/capability_grants"
	"github.com/aurelion-solutions/backplane/internal/inventory/capability_mappings"
	"github.com/google/uuid"
)

// projectInput bundles everything a single (record, mapping) pair
// needs to project. Kept package-private — the action assembles it.
type projectInput struct {
	GrantExternalID string         // record.external_id
	Resource        string         // payload.resource
	ResourceKind    string         // payload.resource_kind
	ActionSlug      string         // payload.action_slug
	Account         *accounts.Account
	Mapping         *capability_mappings.CapabilityMapping
	Now             time.Time
}

// project runs one mapping against one grant record. Returns (grant,
// true) when the mapping matches and a CapabilityGrant should be
// upserted, or (nil, false) when this mapping does not apply to this
// record.
//
// Pure function — no IO, no state. The caller decides what to do
// with the result.
func project(in projectInput) (*capability_grants.CapabilityGrant, bool) {
	m := in.Mapping

	// 1. application_id filter.
	if m.ApplicationID != nil && *m.ApplicationID != in.Account.ApplicationID {
		return nil, false
	}
	// 2. action_slug filter.
	if m.ActionSlug != nil && *m.ActionSlug != in.ActionSlug {
		return nil, false
	}
	// 3. Resource match — XOR over (resource_id, resource_kind, resource_path_glob).
	if !resourceMatches(m, in.Resource, in.ResourceKind) {
		return nil, false
	}
	// 4. Resolve scope value.
	scopeValue, ok := resolveScopeValue(m.ScopeValueSource, in.Account, in.Resource)
	if !ok {
		return nil, false
	}

	grant := &capability_grants.CapabilityGrant{
		ID:                        uuid.New(),
		AccountID:                 in.Account.ID,
		CapabilityID:              m.CapabilityID,
		ScopeKeyID:                m.ScopeKeyID,
		ScopeValue:                scopeValuePtr(scopeValue),
		ApplicationID:             in.Account.ApplicationID,
		SourceGrantExternalID:     in.GrantExternalID,
		SourceCapabilityMappingID: m.ID,
		ObservedAt:                in.Now,
		CreatedAt:                 in.Now,
		UpdatedAt:                 in.Now,
	}
	return grant, true
}

// resourceMatches checks the XOR-selector of the mapping against the
// record's resource and resource_kind. resource_id matching against
// lake records is not implemented (lake records carry the resource
// external_id as a string, not a backplane UUID) — that path returns
// false.
func resourceMatches(m *capability_mappings.CapabilityMapping, resource, resourceKind string) bool {
	switch {
	case m.ResourceID != nil:
		// Resources don't live in PG yet; cannot match by id.
		return false
	case m.ResourceKind != nil:
		return *m.ResourceKind == resourceKind
	case m.ResourcePathGlob != nil:
		matched, err := path.Match(*m.ResourcePathGlob, resource)
		return err == nil && matched
	}
	return false
}

// resolveScopeValue evaluates the scope_value_source JSONB
// discriminated union. Empty string ⇒ GLOBAL (NULL scope_value).
//
// Supported kinds:
//   - constant:             { "kind": "constant", "value": "<string>" }
//   - application_id:       { "kind": "application_id" }
//   - principal_attribute:  { "kind": "principal_attribute", "key": "<string>" }
//     — pulls a value off the resolved Account's attrs sidecar.
//     "principal" is the right word here: by the time this fires the
//     account has been resolved to a Principal-bound identity, and
//     these attrs typically describe that principal.
//   - resource_external_id: { "kind": "resource_external_id" } — pulls
//     the resource external_id straight off the grant record (no lake
//     lookup needed; the value is already on the projector input).
//     This is the common case for "scope IS the thing" mappings:
//     AD group_member → group SID, ACL file_access → share id,
//     SAP role_assignment → role code, etc.
//
// Not supported yet:
//   - resource_attribute (requires lake lookup into AccessArtifact)
//
// Returns (value, true) on success; (_, false) when the source kind
// is unknown or the required input is missing.
func resolveScopeValue(src map[string]any, acc *accounts.Account, resource string) (string, bool) {
	kind, _ := src["kind"].(string)
	switch kind {
	case "constant":
		v, _ := src["value"].(string)
		return v, true
	case "application_id":
		return acc.ApplicationID.String(), true
	case "principal_attribute":
		key, _ := src["key"].(string)
		if key == "" {
			return "", false
		}
		if v, ok := acc.Attrs[key].(string); ok {
			return v, true
		}
		return "", false
	case "resource_external_id":
		if resource == "" {
			return "", false
		}
		return resource, true
	case "resource_attribute":
		// Not implemented — lake lookup into AccessArtifact required.
		return "", false
	}
	return "", false
}

// scopeValuePtr returns NULL for empty strings (GLOBAL) and a pointer
// otherwise.
func scopeValuePtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

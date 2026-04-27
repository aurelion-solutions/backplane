// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package capability_mappings owns the CapabilityMapping rule set —
// admin-written rules that translate raw access facts
// (AccessGrantRecord) into business capabilities (CapabilityGrant).
//
// Each rule is a filter: one Capability + one ScopeKey + a resource
// selector (XOR over resource_id / resource_kind / resource_path_glob)
// + optional application_id / action_slug filter. scope_value_source
// is a JSONB discriminated union describing how to compute the scope
// value at projection time:
//
//   { "kind": "constant",             "value": "<string>" }
//   { "kind": "application_id" }
//   { "kind": "principal_attribute",  "key":  "<attr key on Account>" }
//   { "kind": "resource_external_id" } — straight from the grant record
//   { "kind": "resource_attribute",   "key":  "<attr key on AccessArtifact>" }   (lake lookup; not implemented yet)
package capability_mappings

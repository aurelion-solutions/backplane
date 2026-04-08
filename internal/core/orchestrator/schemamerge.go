// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/grammar"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
)

// BuildMergedSchema returns a fresh copy of the embedded pipeline
// grammar with per-action arg / result schemas injected under
// $defs.action_args[<engine>.<action>] and $defs.action_results[…].
//
// Merge is ADDITIVE — existing $defs entries are not overwritten.
// External consumers (the VSCode YAML completion in
// aurelion-engineering-studio is the canonical client) must read
// per-action schemas under these two keys.
func BuildMergedSchema(reg *registry.Registry) (map[string]any, error) {
	out, err := grammar.Parsed()
	if err != nil {
		return nil, err
	}
	defs, _ := out["$defs"].(map[string]any)
	if defs == nil {
		defs = map[string]any{}
		out["$defs"] = defs
	}
	actionArgs, _ := defs["action_args"].(map[string]any)
	if actionArgs == nil {
		actionArgs = map[string]any{}
		defs["action_args"] = actionArgs
	}
	actionResults, _ := defs["action_results"].(map[string]any)
	if actionResults == nil {
		actionResults = map[string]any{}
		defs["action_results"] = actionResults
	}
	if reg != nil {
		for _, entry := range reg.All() {
			key := entry.Engine + "." + entry.Action
			actionArgs[key] = entry.ArgsSchema
			actionResults[key] = entry.ResultSchema
		}
	}
	return out, nil
}

// ActionDescriptor is the JSON shape returned by GET /api/v0/actions
// (catalogue view of every registered action).
type ActionDescriptor struct {
	Engine       string         `json:"engine"`
	Action       string         `json:"action"`
	Idempotent   bool           `json:"idempotent"`
	ArgsSchema   map[string]any `json:"args_schema"`
	ResultSchema map[string]any `json:"result_schema"`
}

// BuildActionCatalogue returns a sorted slice of action descriptors
// for the well-known catalogue endpoint.
func BuildActionCatalogue(reg *registry.Registry) []ActionDescriptor {
	if reg == nil {
		return []ActionDescriptor{}
	}
	entries := reg.All()
	out := make([]ActionDescriptor, 0, len(entries))
	for _, e := range entries {
		out = append(out, ActionDescriptor{
			Engine:       e.Engine,
			Action:       e.Action,
			Idempotent:   e.Idempotent,
			ArgsSchema:   e.ArgsSchema,
			ResultSchema: e.ResultSchema,
		})
	}
	return out
}

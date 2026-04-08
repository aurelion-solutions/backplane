// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package matcher

import (
	"strings"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
)

// PayloadSatisfiesMatch reports whether every (key, value) in match
// is present and containment-compatible in payload.
//
//   - Nested maps recurse with the same rule.
//   - Primitive lists use set-containment: every element of match
//     must appear in payload; order is ignored.
//   - Scalars use plain equality.
//
// Empty match returns true (matches any payload). This mirrors the
// Postgres JSONB `match <@ payload` semantics used by
// Service.FindMatchingWaiterStepIDs.
func PayloadSatisfiesMatch(match, payload map[string]any) bool {
	if len(match) == 0 {
		return true
	}
	for k, mv := range match {
		pv, ok := payload[k]
		if !ok {
			return false
		}
		if !valueSatisfies(mv, pv) {
			return false
		}
	}
	return true
}

func valueSatisfies(matchVal, payloadVal any) bool {
	switch mv := matchVal.(type) {
	case map[string]any:
		pv, ok := payloadVal.(map[string]any)
		if !ok {
			return false
		}
		return PayloadSatisfiesMatch(mv, pv)
	case []any:
		pv, ok := payloadVal.([]any)
		if !ok {
			return false
		}
		for _, item := range mv {
			found := false
			for _, candidate := range pv {
				if item == candidate {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	default:
		return matchVal == payloadVal
	}
}

// MatchingMQTrigger is one (definition, trigger) pair selected by
// FindMatchingMQTriggers — the matcher fires a CreateRun for each.
type MatchingMQTrigger struct {
	Definition *loader.Definition
	Trigger    map[string]any
}

// FindMatchingMQTriggers walks every pipeline definition and selects
// the mq triggers whose routing_key equals routingKey AND whose match
// predicate is contained in payload.
func FindMatchingMQTriggers(
	defs []*loader.Definition,
	routingKey string,
	payload map[string]any,
) []MatchingMQTrigger {
	out := []MatchingMQTrigger{}
	for _, defn := range defs {
		for _, trigger := range defn.Triggers {
			if t, _ := trigger["type"].(string); t != "mq" {
				continue
			}
			if rk, _ := trigger["routing_key"].(string); rk != routingKey {
				continue
			}
			matchSpec, _ := trigger["match"].(map[string]any)
			if !PayloadSatisfiesMatch(matchSpec, payload) {
				continue
			}
			out = append(out, MatchingMQTrigger{Definition: defn, Trigger: trigger})
		}
	}
	return out
}

// ExtractArgsFromPayload resolves the dotted-path mapping declared by
// an mq trigger's args_from_payload field. Missing paths yield nil
// (not an error) — mirroring kernel behaviour.
func ExtractArgsFromPayload(spec map[string]any, payload map[string]any) map[string]any {
	out := map[string]any{}
	for argName, raw := range spec {
		dottedPath, ok := raw.(string)
		if !ok {
			continue
		}
		out[argName] = walkDotted(payload, dottedPath)
	}
	return out
}

func walkDotted(root any, dotted string) any {
	parts := strings.Split(dotted, ".")
	node := root
	for _, p := range parts {
		m, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		v, ok := m[p]
		if !ok {
			return nil
		}
		node = v
	}
	return node
}

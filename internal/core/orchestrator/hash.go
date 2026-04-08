// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// ContentHash returns sha256(canonical_json(args)) as a hex string.
// Used to populate the partial-UNIQUE idempotency index on
// pipeline_runs and to detect arg drift on retry.
//
// Canonicalisation: keys are sorted at every level so map-iteration
// order does not affect the hash.
func ContentHash(args map[string]any) string {
	body, _ := json.Marshal(canonicalize(args))
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func canonicalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]kv, 0, len(keys))
		for _, k := range keys {
			out = append(out, kv{K: k, V: canonicalize(t[k])})
		}
		return sortedMap(out)
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = canonicalize(child)
		}
		return out
	default:
		return v
	}
}

type kv struct {
	K string
	V any
}

type sortedMap []kv

func (s sortedMap) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, len(s))
	for _, e := range s {
		out[e.K] = e.V
	}
	return json.Marshal(out)
}

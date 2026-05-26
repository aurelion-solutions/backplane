// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package authzen

import (
	"fmt"
	"sort"
)

// deriveFacets builds the facet slice used for store pre-filtering.
//
// Built-in facets always present:
//
//	"authz"
//	"action:<action.name>"
//	"resource:<resource.type>"
//	"principal:<subject.type>"  — AuthZen wire calls it "subject", Aurelion contract calls the actor "principal"
//
// Caller-supplied facets come from request.Context — every scalar entry
// becomes "<key>:<value>"; nested map values are flattened one level
// (`{"geo":{"country":"DE","region":"eu"}}` →
// `["geo:country:DE", "geo:region:eu"]`).
//
// Slice values produce one facet per element under the same key.
// Anything else is JSON-stringified.
func deriveFacets(req Request) []string {
	out := []string{"authz"}
	if req.Action.Name != "" {
		out = append(out, "action:"+req.Action.Name)
	}
	if req.Resource.Type != "" {
		out = append(out, "resource:"+req.Resource.Type)
	}
	if req.Subject.Type != "" {
		out = append(out, "principal:"+req.Subject.Type)
	}
	for k, v := range req.Context {
		out = append(out, flatten(k, v)...)
	}
	sort.Strings(out)
	return dedupe(out)
}

func flatten(prefix string, v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return []string{prefix + ":" + x}
	case bool:
		return []string{prefix + ":" + fmt.Sprintf("%v", x)}
	case float64:
		// JSON numbers always decode as float64; preserve int form when
		// the value is integral so tags like "depth:3" stay readable.
		if x == float64(int64(x)) {
			return []string{fmt.Sprintf("%s:%d", prefix, int64(x))}
		}
		return []string{fmt.Sprintf("%s:%v", prefix, x)}
	case int:
		return []string{fmt.Sprintf("%s:%d", prefix, x)}
	case int64:
		return []string{fmt.Sprintf("%s:%d", prefix, x)}
	case []any:
		out := []string{}
		for _, item := range x {
			out = append(out, flatten(prefix, item)...)
		}
		return out
	case map[string]any:
		out := []string{}
		for k, val := range x {
			out = append(out, flatten(prefix+":"+k, val)...)
		}
		return out
	default:
		return []string{prefix + ":" + fmt.Sprintf("%v", x)}
	}
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

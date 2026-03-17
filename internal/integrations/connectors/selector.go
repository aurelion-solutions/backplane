// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

// Matching returns every instance whose tag set is a superset of
// requiredTags. An empty requiredTags matches every instance. Pure
// function — no I/O, no clock, no allocation beyond the result slice.
func Matching(instances []*ConnectorInstance, requiredTags []string) []*ConnectorInstance {
	required := toSet(requiredTags)
	out := make([]*ConnectorInstance, 0, len(instances))
	for _, inst := range instances {
		if isSuperset(toSet(inst.Tags), required) {
			out = append(out, inst)
		}
	}
	return out
}

// Pick returns the first instance whose tag set is a superset of
// requiredTags, or nil. Same rule as Matching; returns nil rather than
// an empty slice so callers can branch concisely.
func Pick(instances []*ConnectorInstance, requiredTags []string) *ConnectorInstance {
	matched := Matching(instances, requiredTags)
	if len(matched) == 0 {
		return nil
	}
	return matched[0]
}

func toSet(values []string) map[string]struct{} {
	s := make(map[string]struct{}, len(values))
	for _, v := range values {
		s[v] = struct{}{}
	}
	return s
}

func isSuperset(actual, required map[string]struct{}) bool {
	for k := range required {
		if _, ok := actual[k]; !ok {
			return false
		}
	}
	return true
}

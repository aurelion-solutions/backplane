// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import "encoding/json"

// descriptorToMap re-encodes a CapabilityDescriptor as a generic JSON
// object so the repository can persist it as JSONB without bun trying
// to introspect the typed struct as a column-level shape.
func descriptorToMap(d CapabilityDescriptor) map[string]any {
	raw, _ := json.Marshal(d)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

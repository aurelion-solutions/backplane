// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest is the in-memory projection of one <rule>.meta.json sidecar
// that accompanies a <rule>.rego file inside a cartridge.
//
// Fields mirror the kernel CartridgeManifest exactly. Mechanism is a
// plain string at this layer — the platform doesn't know domain enums
// like PolicyMechanism; consumers (engines) validate against their own
// vocabulary.
type Manifest struct {
	RuleID                string         `json:"rule_id"`
	Version               int            `json:"version"`
	Name                  string         `json:"name"`
	Description           string         `json:"description,omitempty"`
	Mechanism             string         `json:"mechanism"`
	Finding               map[string]any `json:"finding,omitempty"`
	HumanizeTemplate      string         `json:"humanize_template,omitempty"`
	DefaultRecommendation string         `json:"default_recommendation,omitempty"`
}

// loadManifest reads one .meta.json sidecar and validates the minimum
// required fields. RuleID, Version, Name, Mechanism are mandatory.
func loadManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: read %q: %v", ErrInvalidManifest, path, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: parse %q: %v", ErrInvalidManifest, path, err)
	}
	if m.RuleID == "" {
		return Manifest{}, fmt.Errorf("%w: %q: rule_id is required", ErrInvalidManifest, path)
	}
	if m.Version == 0 {
		return Manifest{}, fmt.Errorf("%w: %q: version is required", ErrInvalidManifest, path)
	}
	if m.Name == "" {
		return Manifest{}, fmt.Errorf("%w: %q: name is required", ErrInvalidManifest, path)
	}
	if m.Mechanism == "" {
		return Manifest{}, fmt.Errorf("%w: %q: mechanism is required", ErrInvalidManifest, path)
	}
	return m, nil
}

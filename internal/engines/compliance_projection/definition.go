// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// projectionFile is the conventional filename a projection cartridge
// places at its root. A cartridge without this file is not a projection
// cartridge and is skipped during discovery.
const projectionFile = "projection.json"

// Definition is the declarative projection a cartridge carries: a named
// external projection plus its controls and the finding kinds that
// violate each. It carries NO policies — the inversion is deliberate
// (a compliance cartridge is mapping + templates over the single source
// of a finding, never a duplicate evaluator).
type Definition struct {
	Projection     string    `json:"projection"`
	Name           string    `json:"name"`
	Type           string    `json:"type,omitempty"`
	CriteriaSource string    `json:"criteria_source,omitempty"`
	Description    string    `json:"description,omitempty"`
	Disclaimer     string    `json:"disclaimer,omitempty"`
	Controls       []Control `json:"controls"`
}

// Control is one external control and the posture finding kinds whose
// presence violates it.
//
// Population is descriptive scope metadata — the finding target_types the
// control concerns. It is surfaced in the control definition (returned in
// the control detail) for context, but the coverage computation does NOT
// read it: violations, gaps, and the evaluated signal derive from finding
// kinds and the rule→kind map (see coverage.go). Blind spots are
// attributed precisely through that map, not guessed from a target type.
type Control struct {
	ControlID      string   `json:"control_id"`
	Title          string   `json:"title"`
	Criteria       string   `json:"criteria,omitempty"`
	Category       string   `json:"category,omitempty"`
	ViolatingKinds []string `json:"violating_kinds"`
	Population     []string `json:"population,omitempty"`
}

// control looks up a control by id; ok is false when absent.
func (d Definition) control(controlID string) (Control, bool) {
	for _, c := range d.Controls {
		if c.ControlID == controlID {
			return c, true
		}
	}
	return Control{}, false
}

// loadDefinitions walks every cartridge the source can serve and returns
// the projection definitions among them (cartridges carrying a
// projection.json at their root). A cartridge without the file is not an
// error — it is simply not a projection.
func loadDefinitions(src CartridgeReader) ([]Definition, error) {
	refs, err := src.List()
	if err != nil {
		return nil, fmt.Errorf("compliance_projection: list cartridges: %w", err)
	}
	out := make([]Definition, 0, len(refs))
	for _, ref := range refs {
		root, err := src.Materialize(ref)
		if err != nil {
			return nil, fmt.Errorf("compliance_projection: materialize %q: %w", ref.ID, err)
		}
		def, ok, err := readProjection(root)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, def)
		}
	}
	return out, nil
}

// loadDefinition returns the single projection definition declaring the
// given projection id, or ErrProjectionNotFound.
func loadDefinition(src CartridgeReader, projection string) (Definition, error) {
	defs, err := loadDefinitions(src)
	if err != nil {
		return Definition{}, err
	}
	for _, d := range defs {
		if d.Projection == projection {
			return d, nil
		}
	}
	return Definition{}, fmt.Errorf("%w: %q", ErrProjectionNotFound, projection)
}

// readProjection reads and validates the projection.json at a
// materialized cartridge root. ok is false (no error) when the cartridge
// has no projection.json.
func readProjection(root string) (Definition, bool, error) {
	path := filepath.Join(root, projectionFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Definition{}, false, nil
		}
		return Definition{}, false, fmt.Errorf("%w: read %q: %v", ErrInvalidDefinition, path, err)
	}
	var d Definition
	if err := json.Unmarshal(raw, &d); err != nil {
		return Definition{}, false, fmt.Errorf("%w: parse %q: %v", ErrInvalidDefinition, path, err)
	}
	if d.Projection == "" {
		return Definition{}, false, fmt.Errorf("%w: %q: projection is required", ErrInvalidDefinition, path)
	}
	if d.Name == "" {
		return Definition{}, false, fmt.Errorf("%w: %q: name is required", ErrInvalidDefinition, path)
	}
	if len(d.Controls) == 0 {
		return Definition{}, false, fmt.Errorf("%w: %q: at least one control is required", ErrInvalidDefinition, path)
	}
	return d, true, nil
}

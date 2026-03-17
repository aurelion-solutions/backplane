// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package applications

import (
	"fmt"
	"regexp"
	"strings"
)

// codeRe enforces a kebab/snake-friendly slug pattern: starts with
// lower-alnum, rest may include lower-alnum, underscore, hyphen.
// Matches kernel's CODE_PATTERN byte-for-byte.
var codeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// CreatePayload is the POST /applications body.
type CreatePayload struct {
	Name                  string         `json:"name"`
	Code                  string         `json:"code"`
	Config                map[string]any `json:"config,omitempty"`
	RequiredConnectorTags []string       `json:"required_connector_tags,omitempty"`
	IsActive              *bool          `json:"is_active,omitempty"`
}

// Validate enforces the wire contract before the service layer runs.
func (p CreatePayload) Validate() error {
	name := strings.TrimSpace(p.Name)
	if name == "" || len(name) > 255 {
		return fmt.Errorf("applications: name must be 1..255 characters")
	}
	if !codeRe.MatchString(p.Code) {
		return fmt.Errorf("applications: code must match %s", codeRe.String())
	}
	return nil
}

// PatchPayload is the PATCH /applications/{id} body. Every field is
// optional; the service rejects a payload with no fields set.
type PatchPayload struct {
	Name                  *string        `json:"name,omitempty"`
	Code                  *string        `json:"code,omitempty"`
	Config                map[string]any `json:"config,omitempty"`
	RequiredConnectorTags []string       `json:"required_connector_tags,omitempty"`
	IsActive              *bool          `json:"is_active,omitempty"`
}

// HasAny reports whether at least one mutable field is set.
func (p PatchPayload) HasAny() bool {
	return p.Name != nil || p.Code != nil || p.Config != nil || p.RequiredConnectorTags != nil || p.IsActive != nil
}

// Validate enforces field constraints on the present fields only.
func (p PatchPayload) Validate() error {
	if !p.HasAny() {
		return ErrNoFields
	}
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" || len(name) > 255 {
			return fmt.Errorf("applications: name must be 1..255 characters")
		}
	}
	if p.Code != nil && !codeRe.MatchString(*p.Code) {
		return fmt.Errorf("applications: code must match %s", codeRe.String())
	}
	return nil
}

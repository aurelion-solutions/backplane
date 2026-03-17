// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package applications

import (
	"errors"
	"strings"
	"testing"
)

func TestCreatePayload_Validate_BadCode(t *testing.T) {
	cases := []string{"", "Bad", "starts-with-hyphen-?", strings.Repeat("a", 65)}
	for _, code := range cases {
		t.Run(code, func(t *testing.T) {
			p := CreatePayload{Name: "ok", Code: code}
			if err := p.Validate(); err == nil {
				t.Fatalf("expected validation error for code %q", code)
			}
		})
	}
}

func TestCreatePayload_Validate_BadName(t *testing.T) {
	cases := []string{"", "   ", strings.Repeat("n", 256)}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			p := CreatePayload{Name: name, Code: "ok"}
			if err := p.Validate(); err == nil {
				t.Fatalf("expected validation error for name %q", name)
			}
		})
	}
}

func TestCreatePayload_Validate_OK(t *testing.T) {
	p := CreatePayload{Name: "Salesforce Prod", Code: "salesforce-prod"}
	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchPayload_Validate_RejectsEmpty(t *testing.T) {
	p := PatchPayload{}
	if err := p.Validate(); !errors.Is(err, ErrNoFields) {
		t.Fatalf("want ErrNoFields, got %v", err)
	}
}

func TestPatchPayload_Validate_BadName(t *testing.T) {
	name := "   "
	p := PatchPayload{Name: &name}
	if err := p.Validate(); err == nil {
		t.Fatalf("expected validation error for empty name")
	}
}

func TestPatchPayload_HasAny(t *testing.T) {
	p := PatchPayload{}
	if p.HasAny() {
		t.Fatalf("empty patch must report HasAny=false")
	}
	tags := []string{"x"}
	p.RequiredConnectorTags = tags
	if !p.HasAny() {
		t.Fatalf("patch with tags must report HasAny=true")
	}
}

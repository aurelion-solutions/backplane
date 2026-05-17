// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package descriptor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

// adApp builds the canonical Microsoft AD-style descriptor used
// across happy-path tests. Mirrors the popular/apps/microsoft_ad
// cartridge shape closely enough that any drift here is caught early.
func adApp() cartridges.AppCartridge {
	return cartridges.AppCartridge{
		ID: "microsoft_ad",
		Account: cartridges.AccountStateMachine{
			States: []string{"not_exist", "active", "blocked", "invited"},
		},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"userPrincipalName": {
					Template:    "{{ .Principal.Firstname | lower }}.{{ .Principal.Lastname | lower }}@{{ .Application.Domain }}",
					Transforms:  []string{"remove_diacritics"},
					OnCollision: "username_numeric_suffix",
				},
				"samAccountName": {
					Template:   "{{ .Principal.Firstname | lower }}.{{ .Principal.Lastname | lower }}",
					Transforms: []string{"remove_diacritics", "truncate:8"},
				},
				"displayName": {
					Template: "{{ .Principal.Firstname }} {{ .Principal.Lastname }}",
				},
				"ou": {
					ByState: map[string]any{
						"active":  "OU={{ .Principal.OrgUnit }},OU=Users,{{ .Application.BaseDN }}",
						"blocked": "OU=Disabled,{{ .Application.BaseDN }}",
						"invited": "OU={{ .Principal.OrgUnit }},OU=Pending,{{ .Application.BaseDN }}",
					},
				},
				"distinguishedName": {
					Template: "CN={{ .Descriptor.displayName }},{{ .Descriptor.ou }}",
				},
				"userAccountControl": {
					ByState: map[string]any{
						"active":  512,
						"blocked": 514,
						"invited": 546,
					},
				},
			},
		},
	}
}

func adInputs(state string) Inputs {
	return Inputs{
		Principal: map[string]any{
			"Firstname": "Iván",
			"Lastname":  "Müller-Schmidt",
			"OrgUnit":   "engineering",
		},
		Application: map[string]any{
			"Domain": "corp.example.com",
			"BaseDN": "DC=corp,DC=example,DC=com",
		},
		TargetState: state,
	}
}

func TestRender_Active(t *testing.T) {
	r, err := NewRenderer(adApp(), nil)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	res, err := r.Render(context.Background(), adInputs("active"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	checks := map[string]any{
		"userPrincipalName":  "ivan.muller-schmidt@corp.example.com",
		"samAccountName":     "ivan.mul",
		"displayName":        "Iván Müller-Schmidt",
		"ou":                 "OU=engineering,OU=Users,DC=corp,DC=example,DC=com",
		"distinguishedName":  "CN=Iván Müller-Schmidt,OU=engineering,OU=Users,DC=corp,DC=example,DC=com",
		"userAccountControl": 512,
	}
	for k, want := range checks {
		got, ok := res.Fields[k]
		if !ok {
			t.Errorf("missing field %q in %v", k, res.Fields)
			continue
		}
		if got != want {
			t.Errorf("field %q = %v (%T), want %v (%T)", k, got, got, want, want)
		}
	}
}

func TestRender_Blocked_OmitsOrgUnit(t *testing.T) {
	r, err := NewRenderer(adApp(), nil)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	res, err := r.Render(context.Background(), adInputs("blocked"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := res.Fields["ou"]; got != "OU=Disabled,DC=corp,DC=example,DC=com" {
		t.Errorf("ou = %v, want OU=Disabled,...", got)
	}
	if got := res.Fields["userAccountControl"]; got != 514 {
		t.Errorf("uac = %v, want 514", got)
	}
}

func TestRender_NotExist_OmitsByStateFields(t *testing.T) {
	// `not_exist` has no entry in any by_state map for ad — those
	// fields should be missing from the result entirely.
	r, err := NewRenderer(adApp(), nil)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	in := adInputs("not_exist")
	res, err := r.Render(context.Background(), in)
	if err != nil {
		// distinguishedName references .Descriptor.ou which isn't set
		// for not_exist → strict missingkey makes this an error. That
		// is the intended contract.
		if !strings.Contains(err.Error(), "ou") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if _, ok := res.Fields["ou"]; ok {
		t.Errorf("ou should be omitted for not_exist target state, got %v", res.Fields["ou"])
	}
	if _, ok := res.Fields["userAccountControl"]; ok {
		t.Errorf("userAccountControl should be omitted, got %v", res.Fields["userAccountControl"])
	}
}

func TestRender_CrossField_OrderEnforced(t *testing.T) {
	// Cross-field reference (distinguishedName → displayName, ou) must
	// resolve regardless of map iteration order. Build the recipe
	// with a single descriptor field that only references via
	// .Descriptor and verify it renders the right values.
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"third":  {Template: "{{ .Descriptor.first }}-{{ .Descriptor.second }}"},
				"first":  {Template: "A"},
				"second": {Template: "B"},
			},
		},
	}
	r, err := NewRenderer(app, nil)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	res, err := r.Render(context.Background(), Inputs{TargetState: "x"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := res.Fields["third"]; got != "A-B" {
		t.Fatalf("third = %v, want A-B", got)
	}
}

func TestRender_CycleRejected(t *testing.T) {
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {Template: "{{ .Descriptor.b }}"},
				"b": {Template: "{{ .Descriptor.a }}"},
			},
		},
	}
	_, err := NewRenderer(app, nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestRender_UndeclaredRefRejected(t *testing.T) {
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {Template: "{{ .Descriptor.nope }}"},
			},
		},
	}
	_, err := NewRenderer(app, nil)
	if err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("want undeclared-field error, got %v", err)
	}
}

func TestRender_UnknownTransformRejected(t *testing.T) {
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {Template: "x", Transforms: []string{"nonsense"}},
			},
		},
	}
	_, err := NewRenderer(app, nil)
	if err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Fatalf("want unknown-transform error, got %v", err)
	}
}

func TestRender_BadTemplateRejected(t *testing.T) {
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {Template: "{{ .Principal.X "},
			},
		},
	}
	_, err := NewRenderer(app, nil)
	if err == nil {
		t.Fatalf("want template parse error")
	}
}

func TestRender_TruncateUnicodeSafe(t *testing.T) {
	// Truncating to 4 runes from "Ivánka" must NOT split the í/á
	// multibyte sequences. Expected result after remove_diacritics is
	// "Ivan".
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {
					Template:   "{{ .Principal.Name }}",
					Transforms: []string{"remove_diacritics", "truncate:4"},
				},
			},
		},
	}
	r, err := NewRenderer(app, nil)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	res, err := r.Render(context.Background(), Inputs{
		Principal:   map[string]any{"Name": "Ivánka"},
		TargetState: "x",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := res.Fields["a"]; got != "Ivan" {
		t.Errorf("a = %q, want Ivan", got)
	}
}

func TestRender_ResolverRegistry_Numeric_Suffix(t *testing.T) {
	// Stub a resolver that appends "2" to the value to confirm
	// on_collision is actually invoked and its output replaces the
	// pre-collision string.
	resolvers := ResolverRegistry{
		"append_2": ResolverFunc(func(_ context.Context, in ResolverInput) (string, error) {
			return in.Value + "2", nil
		}),
	}
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {Template: "alice", OnCollision: "append_2"},
			},
		},
	}
	r, err := NewRenderer(app, resolvers)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	res, err := r.Render(context.Background(), Inputs{TargetState: "x"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := res.Fields["a"]; got != "alice2" {
		t.Errorf("a = %v, want alice2", got)
	}
}

func TestRender_UnknownResolver_FallsBackToStub(t *testing.T) {
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {Template: "alice", OnCollision: "never_registered"},
			},
		},
	}
	r, err := NewRenderer(app, ResolverRegistry{})
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	res, err := r.Render(context.Background(), Inputs{TargetState: "x"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := res.Fields["a"]; got != "alice" {
		t.Errorf("a = %v, want alice (stub passes through)", got)
	}
}

func TestRender_ResolverError(t *testing.T) {
	sentinel := errors.New("storage down")
	resolvers := ResolverRegistry{
		"boom": ResolverFunc(func(_ context.Context, _ ResolverInput) (string, error) {
			return "", sentinel
		}),
	}
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {Template: "x", OnCollision: "boom"},
			},
		},
	}
	r, err := NewRenderer(app, resolvers)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	_, err = r.Render(context.Background(), Inputs{TargetState: "x"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
}

func TestRender_MissingPrincipalField_Strict(t *testing.T) {
	// missingkey=error means a reference to a key not in the principal
	// map fails at render time rather than silently rendering
	// "<no value>".
	app := cartridges.AppCartridge{
		Account: cartridges.AccountStateMachine{States: []string{"x"}},
		Descriptor: cartridges.Descriptor{
			Fields: map[string]cartridges.DescriptorField{
				"a": {Template: "{{ .Principal.NotThere }}"},
			},
		},
	}
	r, err := NewRenderer(app, nil)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	_, err = r.Render(context.Background(), Inputs{
		Principal:   map[string]any{"Other": "x"},
		TargetState: "x",
	})
	if err == nil {
		t.Fatalf("expected strict missingkey error")
	}
}

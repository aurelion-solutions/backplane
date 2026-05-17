// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package descriptor

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

const (
	manifestYAML = `
id: microsoft_ad
name: Microsoft AD
version: 0.1.0
connector: ad
config:
  Domain: corp.example.com
  BaseDN: "DC=corp,DC=example,DC=com"
`
	accountYAML = `
states: [not_exist, active, blocked]
initial_state: not_exist
transitions:
  - { from: not_exist, to: active }
  - { from: active,    to: blocked }
`
	descriptorYAML = `
fields:
  userPrincipalName:
    template: "{{ .Principal.Firstname }}.{{ .Principal.Lastname }}@{{ .Application.Domain }}"
    transforms: [lower, remove_diacritics]
  ou:
    by_state:
      active:  "OU={{ .Principal.OrgUnit }},OU=Users,{{ .Application.BaseDN }}"
      blocked: "OU=Disabled,{{ .Application.BaseDN }}"
  userAccountControl:
    by_state:
      active:  512
      blocked: 514
`
)

// buildRenderRouter materialises a one-app cartridges fixture and
// wires up the descriptor render endpoint on top of it.
func buildRenderRouter(t *testing.T) *echo.Echo {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "popular", "apps", "microsoft_ad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	write("manifest.yaml", manifestYAML)
	write("account.yaml", accountYAML)
	write("descriptor.yaml", descriptorYAML)

	e := echo.New()
	g := e.Group("")
	RegisterRoutes(g, cartridges.NewFilesystemProvider(root))
	return e
}

func doRender(t *testing.T, e *echo.Echo, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/cartridges/popular/apps/microsoft_ad/descriptor",
		bytes.NewReader(raw),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestRoutes_Render_Active(t *testing.T) {
	e := buildRenderRouter(t)

	rec := doRender(t, e, RenderRequest{
		Principal: map[string]any{
			"Firstname": "Iván",
			"Lastname":  "Müller",
			"OrgUnit":   "engineering",
		},
		TargetState: "active",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got RenderResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	wantUPN := "ivan.muller@corp.example.com"
	if got.Fields["userPrincipalName"] != wantUPN {
		t.Errorf("upn = %v, want %s", got.Fields["userPrincipalName"], wantUPN)
	}
	wantOU := "OU=engineering,OU=Users,DC=corp,DC=example,DC=com"
	if got.Fields["ou"] != wantOU {
		t.Errorf("ou = %v, want %s", got.Fields["ou"], wantOU)
	}
	// userAccountControl came back as a number — JSON decodes it as
	// float64.
	if got.Fields["userAccountControl"] != float64(512) {
		t.Errorf("uac = %v (%T), want 512", got.Fields["userAccountControl"], got.Fields["userAccountControl"])
	}
}

func TestRoutes_Render_Blocked(t *testing.T) {
	e := buildRenderRouter(t)

	rec := doRender(t, e, RenderRequest{
		Principal:   map[string]any{"Firstname": "X", "Lastname": "Y", "OrgUnit": "sales"},
		TargetState: "blocked",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got RenderResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Fields["ou"] != "OU=Disabled,DC=corp,DC=example,DC=com" {
		t.Errorf("ou = %v", got.Fields["ou"])
	}
}

func TestRoutes_Render_ApplicationOverride(t *testing.T) {
	e := buildRenderRouter(t)

	rec := doRender(t, e, RenderRequest{
		Principal: map[string]any{
			"Firstname": "Test",
			"Lastname":  "User",
			"OrgUnit":   "qa",
		},
		Application: map[string]any{"Domain": "staging.example.com"},
		TargetState: "active",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got RenderResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Fields["userPrincipalName"] != "test.user@staging.example.com" {
		t.Errorf("upn = %v (override should win)", got.Fields["userPrincipalName"])
	}
	// BaseDN not overridden — manifest value still in effect.
	if got.Fields["ou"] != "OU=qa,OU=Users,DC=corp,DC=example,DC=com" {
		t.Errorf("ou = %v (BaseDN from manifest)", got.Fields["ou"])
	}
}

func TestRoutes_Render_MissingTargetState(t *testing.T) {
	e := buildRenderRouter(t)

	rec := doRender(t, e, RenderRequest{
		Principal: map[string]any{"Firstname": "x"},
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRoutes_Render_UnknownApp(t *testing.T) {
	e := buildRenderRouter(t)

	body, _ := json.Marshal(RenderRequest{
		Principal:   map[string]any{"Firstname": "x"},
		TargetState: "active",
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/cartridges/popular/apps/nonsense/descriptor",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRoutes_Render_BadPrincipal_StrictMissingKey(t *testing.T) {
	e := buildRenderRouter(t)

	// Principal lacks Firstname — strict template execution should
	// surface a 422.
	rec := doRender(t, e, RenderRequest{
		Principal:   map[string]any{"Lastname": "only"},
		TargetState: "active",
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
}

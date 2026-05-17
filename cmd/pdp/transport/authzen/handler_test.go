// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package authzen

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	cedarmech "github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/mechanisms/cedar"
	"github.com/labstack/echo/v4"
)

const (
	allowPolicy = `permit (
    principal,
    action == Action::"view",
    resource == Document::"doc-42"
);
`
	denyForInactive = `forbid (
    principal,
    action == Action::"view",
    resource
) when {
    principal.is_active == false
};
`
)

// buildCartridgeFixture writes two rules sharing the same bucket: an
// allow + a deny. Both are tagged authn/authz=authz so they get picked
// up under the "authz" facet.
func buildCartridgeFixture(t *testing.T) (cartridges.Provider, string) {
	t.Helper()
	root := t.TempDir()
	bucket := filepath.Join(root, "demo", "policies", "authz")
	if err := os.MkdirAll(bucket, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	allowMeta := `{
		"rule_id":   "demo.allow_doc42_view",
		"version":   1,
		"name":      "Allow view of doc-42",
		"mechanism": "cedar",
		"tags":      ["authz", "action:view", "resource:Document"]
	}`
	if err := os.WriteFile(filepath.Join(bucket, "allow.meta.json"), []byte(allowMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bucket, "allow.cedar"), []byte(allowPolicy), 0o644); err != nil {
		t.Fatal(err)
	}

	denyMeta := `{
		"rule_id":   "demo.deny_inactive",
		"version":   1,
		"name":      "Deny inactive principals",
		"mechanism": "cedar",
		"tags":      ["authz", "action:view"]
	}`
	if err := os.WriteFile(filepath.Join(bucket, "deny.meta.json"), []byte(denyMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bucket, "deny.cedar"), []byte(denyForInactive), 0o644); err != nil {
		t.Fatal(err)
	}

	return cartridges.NewFilesystemProvider(root), root
}

func setup(t *testing.T) (*echo.Echo, *policy_assessment.Store, *policy_assessment.Dispatcher) {
	t.Helper()
	provider, _ := buildCartridgeFixture(t)
	store := policy_assessment.NewStore()
	if _, err := store.Reload(context.Background(), provider); err != nil {
		t.Fatalf("reload: %v", err)
	}
	dispatcher := policy_assessment.NewDispatcher()
	dispatcher.Register(cedarmech.New())
	if _, errs := dispatcher.PrepareAll(context.Background(), store.All()); len(errs) > 0 {
		t.Fatalf("prepare: %v", errs)
	}
	e := echo.New()
	RegisterRoutes(e.Group(""), Deps{Store: store, Dispatcher: dispatcher})
	return e, store, dispatcher
}

func TestAuthZen_AllowsActivePrincipalOnDoc42(t *testing.T) {
	e, _, _ := setup(t)
	body, _ := json.Marshal(Request{
		Subject: Subject{
			Type: "Account", ID: "alice",
			Properties: map[string]any{"status": "active"},
		},
		Resource: Resource{Type: "Document", ID: "doc-42"},
		Action:   Action{Name: "view"},
	})
	req := httptest.NewRequest(http.MethodPost, "/access/v1/evaluation", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Decision {
		t.Fatalf("expected allow, got %+v", resp)
	}
	// Both rules should have been considered.
	if resp.Context.RulesCount != 2 {
		t.Fatalf("rules_count=%d want 2", resp.Context.RulesCount)
	}
}

func TestAuthZen_DenyWinsOverAllow(t *testing.T) {
	e, _, _ := setup(t)
	body, _ := json.Marshal(Request{
		Subject: Subject{
			Type: "Account", ID: "bob",
			Properties: map[string]any{"status": "disabled"},
		},
		Resource: Resource{Type: "Document", ID: "doc-42"},
		Action:   Action{Name: "view"},
	})
	req := httptest.NewRequest(http.MethodPost, "/access/v1/evaluation", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var resp Response
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Decision {
		t.Fatalf("expected deny-wins, got allow; resp=%+v", resp)
	}
}

func TestAuthZen_TagFilterDropsIrrelevantPolicies(t *testing.T) {
	e, _, _ := setup(t)
	// action:edit does not match either rule's tag set (both require
	// action:view) — both rules skipped, default deny.
	body, _ := json.Marshal(Request{
		Subject:  Subject{Type: "Account", ID: "alice"},
		Resource: Resource{Type: "Document", ID: "doc-42"},
		Action:   Action{Name: "edit"},
	})
	req := httptest.NewRequest(http.MethodPost, "/access/v1/evaluation", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var resp Response
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Decision {
		t.Fatalf("expected default deny on edit, got allow")
	}
	if resp.Context.RulesCount != 0 {
		t.Fatalf("rules_count=%d want 0 (tags filtered them out)", resp.Context.RulesCount)
	}
}

func TestAuthZen_InvalidRequest(t *testing.T) {
	e, _, _ := setup(t)
	// Missing subject.id
	body, _ := json.Marshal(Request{
		Subject:  Subject{Type: "Account"},
		Resource: Resource{Type: "Document", ID: "x"},
		Action:   Action{Name: "view"},
	})
	req := httptest.NewRequest(http.MethodPost, "/access/v1/evaluation", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

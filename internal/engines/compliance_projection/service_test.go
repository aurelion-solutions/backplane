// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	par "github.com/aurelion-solutions/backplane/internal/inventory/policy_assessment_runs"
	peo "github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
)

// --- fakes ---

type fakeCartridges struct {
	root     string
	policies map[string]cartridges.Manifest
}

func (f *fakeCartridges) List() ([]cartridges.Ref, error)            { return []cartridges.Ref{{ID: "soc2"}}, nil }
func (f *fakeCartridges) Materialize(cartridges.Ref) (string, error) { return f.root, nil }
func (f *fakeCartridges) Policies(cartridges.Ref) (map[string]cartridges.Manifest, error) {
	return f.policies, nil
}

type fakeFindings struct{ rows []*findings.Finding }

func (f *fakeFindings) List(_ context.Context, fl findings.ListFilter) ([]*findings.Finding, int, error) {
	out := []*findings.Finding{}
	for _, r := range f.rows {
		if fl.Kind != "" && r.Kind != fl.Kind {
			continue
		}
		out = append(out, r)
	}
	return out, len(out), nil
}

type fakeOutcomes struct {
	rows []*peo.PolicyEvaluationOutcome
}

func (f *fakeOutcomes) List(_ context.Context, _ peo.ListFilter) ([]*peo.PolicyEvaluationOutcome, int, error) {
	return f.rows, len(f.rows), nil
}

type fakeRuns struct{ run *par.AssessmentRun }

func (f *fakeRuns) GetByID(_ context.Context, _ uuid.UUID) (*par.AssessmentRun, error) {
	if f.run == nil {
		return nil, par.ErrNotFound
	}
	return f.run, nil
}

// buildService wires a service over fakes with a one-control SOC2-ish
// projection cartridge written to a temp dir.
func buildService(t *testing.T) (*Service, uuid.UUID) {
	t.Helper()
	root := t.TempDir()
	def := Definition{
		Projection: "soc2-logical-access",
		Name:       "SOC 2 — Logical Access",
		Type:       "attestation",
		Controls: []Control{
			{ControlID: "CC6.1", Title: "Auth", ViolatingKinds: []string{"privileged_access"}},
			{ControlID: "CC6.3", Title: "RBAC", ViolatingKinds: []string{"unused_access"}},
		},
	}
	raw, _ := json.Marshal(def)
	if err := os.WriteFile(filepath.Join(root, projectionFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	runID := uuid.New()
	pid := uuid.New()
	svc, err := NewService(
		&fakeCartridges{
			root: root,
			policies: map[string]cartridges.Manifest{
				"r_priv":   {RuleID: "r_priv", Finding: &cartridges.FindingMeta{Kind: "privileged_access"}},
				"r_unused": {RuleID: "r_unused", Finding: &cartridges.FindingMeta{Kind: "unused_access"}},
			},
		},
		&fakeFindings{rows: []*findings.Finding{
			{ID: uuid.New(), Kind: "privileged_access", Severity: "high", Status: "open", PrincipalID: &pid},
		}},
		&fakeOutcomes{rows: []*peo.PolicyEvaluationOutcome{
			{RuleID: "r_priv", Outcome: peo.OutcomeMatched},
			{RuleID: "r_unused", Outcome: peo.OutcomeNotEvaluable},
		}},
		&fakeRuns{run: &par.AssessmentRun{ID: runID, Status: par.StatusCompleted}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc, runID
}

func TestService_Coverage(t *testing.T) {
	svc, runID := buildService(t)
	rep, err := svc.Coverage(context.Background(), runID, "soc2-logical-access")
	if err != nil {
		t.Fatal(err)
	}
	// CC6.1 → 1 violation, no gap → failed; CC6.3 → 0 violation, 1 gap → not_evaluable.
	if rep.Summary[StateFailed] != 1 || rep.Summary[StateNotEvaluable] != 1 {
		t.Errorf("summary = %+v", rep.Summary)
	}
}

func TestService_UnknownProjection(t *testing.T) {
	svc, runID := buildService(t)
	if _, err := svc.Coverage(context.Background(), runID, "iso-27001"); err == nil {
		t.Fatal("expected ErrProjectionNotFound")
	}
}

func TestRoutes_CoverageAnd404(t *testing.T) {
	svc, runID := buildService(t)
	e := echo.New()
	RegisterRoutes(e.Group("/api/v0"), svc)

	// happy path
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v0/policy-assessment-runs/"+runID.String()+"/projections/soc2-logical-access", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("coverage status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var rep CoverageReport
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Controls) != 2 {
		t.Errorf("controls = %d, want 2", len(rep.Controls))
	}

	// unknown projection → 404
	rec404 := httptest.NewRecorder()
	req404 := httptest.NewRequest(http.MethodGet,
		"/api/v0/policy-assessment-runs/"+runID.String()+"/projections/nope", nil)
	e.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Errorf("unknown projection status = %d, want 404", rec404.Code)
	}
}

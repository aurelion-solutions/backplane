// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment_runs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type memRepo struct {
	rows map[uuid.UUID]*AssessmentRun
}

func newMemRepo() *memRepo { return &memRepo{rows: map[uuid.UUID]*AssessmentRun{}} }

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*AssessmentRun, error) {
	if v, ok := r.rows[id]; ok {
		return v, nil
	}
	return nil, ErrNotFound
}

func (r *memRepo) List(_ context.Context, f ListFilter) ([]*AssessmentRun, int, error) {
	out := []*AssessmentRun{}
	for _, v := range r.rows {
		if f.Status != "" && v.Status != f.Status {
			continue
		}
		if f.TriggeredBy != "" && v.TriggeredBy != f.TriggeredBy {
			continue
		}
		if f.ScopePrincipalID != nil && (v.ScopePrincipalID == nil || *v.ScopePrincipalID != *f.ScopePrincipalID) {
			continue
		}
		if f.ScopeApplicationID != nil && (v.ScopeApplicationID == nil || *v.ScopeApplicationID != *f.ScopeApplicationID) {
			continue
		}
		out = append(out, v)
	}
	total := len(out)
	if f.Offset >= total {
		return []*AssessmentRun{}, total, nil
	}
	out = out[f.Offset:]
	if f.Limit > 0 && f.Limit < len(out) {
		out = out[:f.Limit]
	}
	return out, total, nil
}

func (r *memRepo) Insert(_ context.Context, row *AssessmentRun) error {
	r.rows[row.ID] = row
	return nil
}

func (r *memRepo) Update(_ context.Context, row *AssessmentRun) error {
	if _, ok := r.rows[row.ID]; !ok {
		return ErrNotFound
	}
	r.rows[row.ID] = row
	return nil
}

func mkRun(status, trig string) *AssessmentRun {
	return &AssessmentRun{
		ID:                 uuid.New(),
		Status:             status,
		TriggeredBy:        trig,
		FindingsBySeverity: map[string]int{},
		CreatedAt:          time.Now().UTC(),
	}
}

func TestList_AllAndFiltered(t *testing.T) {
	repo := newMemRepo()
	_ = repo.Insert(context.Background(), mkRun(StatusCompleted, TriggerManual))
	_ = repo.Insert(context.Background(), mkRun(StatusRunning, TriggerSchedule))
	_ = repo.Insert(context.Background(), mkRun(StatusFailed, TriggerSchedule))

	e := echo.New()
	RegisterRoutes(e.Group(""), repo)

	req := httptest.NewRequest(http.MethodGet, "/policy-assessment-runs", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Total != 3 {
		t.Fatalf("total=%d want 3", resp.Total)
	}

	req = httptest.NewRequest(http.MethodGet, "/policy-assessment-runs?triggered_by=schedule", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Fatalf("scheduled total=%d want 2", resp.Total)
	}
}

func TestGet_FoundAndMissing(t *testing.T) {
	repo := newMemRepo()
	run := mkRun(StatusPending, TriggerAPI)
	_ = repo.Insert(context.Background(), run)

	e := echo.New()
	RegisterRoutes(e.Group(""), repo)

	req := httptest.NewRequest(http.MethodGet, "/policy-assessment-runs/"+run.ID.String(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/policy-assessment-runs/"+uuid.New().String(), nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d want 404", rec.Code)
	}
}

func TestGet_InvalidUUID(t *testing.T) {
	e := echo.New()
	RegisterRoutes(e.Group(""), newMemRepo())
	req := httptest.NewRequest(http.MethodGet, "/policy-assessment-runs/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

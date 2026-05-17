// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package findings

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
	rows map[uuid.UUID]*Finding
}

func newMemRepo() *memRepo { return &memRepo{rows: map[uuid.UUID]*Finding{}} }

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Finding, error) {
	if v, ok := r.rows[id]; ok {
		return v, nil
	}
	return nil, ErrNotFound
}

func (r *memRepo) List(_ context.Context, f ListFilter) ([]*Finding, int, error) {
	out := []*Finding{}
	for _, v := range r.rows {
		if f.PrincipalID != nil && (v.PrincipalID == nil || *v.PrincipalID != *f.PrincipalID) {
			continue
		}
		if f.AccountID != nil && (v.AccountID == nil || *v.AccountID != *f.AccountID) {
			continue
		}
		if f.PolicyID != nil && (v.PolicyID == nil || *v.PolicyID != *f.PolicyID) {
			continue
		}
		if f.AssessmentRunID != nil && v.AssessmentRunID != *f.AssessmentRunID {
			continue
		}
		if f.Kind != "" && v.Kind != f.Kind {
			continue
		}
		if f.Status != "" && v.Status != f.Status {
			continue
		}
		if f.Severity != "" && v.Severity != f.Severity {
			continue
		}
		out = append(out, v)
	}
	total := len(out)
	if f.Offset >= total {
		return []*Finding{}, total, nil
	}
	out = out[f.Offset:]
	if f.Limit > 0 && f.Limit < len(out) {
		out = out[:f.Limit]
	}
	return out, total, nil
}

func (r *memRepo) Insert(_ context.Context, row *Finding) error {
	r.rows[row.ID] = row
	return nil
}

func mkFinding(kind, severity, status string, principal *uuid.UUID) *Finding {
	now := time.Now().UTC()
	return &Finding{
		ID:                        uuid.New(),
		AssessmentRunID:           uuid.New(),
		Kind:                      kind,
		PrincipalID:               principal,
		Severity:                  severity,
		Status:                    status,
		MatchedCapabilityGrantIDs: []string{},
		MatchedEffectiveGrantIDs:  []string{},
		MatchedAccessFactIDs:      []string{},
		EvidenceHash:              "h-" + kind,
		DetectedAt:                now,
		EvaluatedAt:               now,
	}
}

func TestList_FilteringByKindSeverityStatus(t *testing.T) {
	repo := newMemRepo()
	alice := uuid.New()
	bob := uuid.New()
	_ = repo.Insert(context.Background(), mkFinding(KindOrphanAccess, SeverityHigh, StatusOpen, &alice))
	_ = repo.Insert(context.Background(), mkFinding(KindSoD, SeverityCritical, StatusOpen, &bob))
	_ = repo.Insert(context.Background(), mkFinding(KindUnusedAccess, SeverityLow, StatusResolved, &alice))

	e := echo.New()
	RegisterRoutes(e.Group(""), repo)

	cases := []struct {
		query string
		want  int
	}{
		{"", 3},
		{"?kind=sod", 1},
		{"?severity=high", 1},
		{"?status=open", 2},
		{"?status=open&severity=critical", 1},
		{"?principal_id=" + alice.String(), 2},
		{"?principal_id=" + bob.String() + "&kind=orphan_access", 0},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/findings"+tc.query, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("query %q status=%d body=%s", tc.query, rec.Code, rec.Body.String())
		}
		var resp ListResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Total != tc.want {
			t.Fatalf("query %q total=%d want %d", tc.query, resp.Total, tc.want)
		}
	}
}

func TestGet_FoundAndMissing(t *testing.T) {
	repo := newMemRepo()
	f := mkFinding(KindOrphanAccess, SeverityHigh, StatusOpen, nil)
	acct := uuid.New()
	f.AccountID = &acct
	_ = repo.Insert(context.Background(), f)

	e := echo.New()
	RegisterRoutes(e.Group(""), repo)

	req := httptest.NewRequest(http.MethodGet, "/findings/"+f.ID.String(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/findings/"+uuid.New().String(), nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d want 404", rec.Code)
	}
}

func TestList_InvalidUUIDQueryParam(t *testing.T) {
	e := echo.New()
	RegisterRoutes(e.Group(""), newMemRepo())
	req := httptest.NewRequest(http.MethodGet, "/findings?principal_id=not-a-uuid", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

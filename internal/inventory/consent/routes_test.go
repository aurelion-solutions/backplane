// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package consent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// -------- fakes (implement both the repository and the lookup) --------

type fakeApps struct {
	list       []*ConsentedApplication
	byID       map[uuid.UUID]*ConsentedApplication
	lastFilter AppListFilter
}

func (f *fakeApps) Upsert(_ context.Context, _ bun.IDB, _ *ConsentedApplication) error { return nil }
func (f *fakeApps) List(_ context.Context, _ bun.IDB, fl AppListFilter) ([]*ConsentedApplication, int, error) {
	f.lastFilter = fl
	return f.list, len(f.list), nil
}
func (f *fakeApps) GetByID(_ context.Context, _ bun.IDB, id uuid.UUID) (*ConsentedApplication, error) {
	if a, ok := f.byID[id]; ok {
		return a, nil
	}
	return nil, ErrNotFound
}

type fakeGrants struct {
	list       []*ConsentGrant
	byID       map[uuid.UUID]*ConsentGrant
	lastFilter GrantListFilter
}

func (f *fakeGrants) Upsert(_ context.Context, _ bun.IDB, _ *ConsentGrant) error { return nil }
func (f *fakeGrants) List(_ context.Context, _ bun.IDB, fl GrantListFilter) ([]*ConsentGrant, int, error) {
	f.lastFilter = fl
	return f.list, len(f.list), nil
}
func (f *fakeGrants) GetByID(_ context.Context, _ bun.IDB, id uuid.UUID) (*ConsentGrant, error) {
	if g, ok := f.byID[id]; ok {
		return g, nil
	}
	return nil, ErrNotFound
}

func newTestServer(apps *fakeApps, grants *fakeGrants) *echo.Echo {
	e := echo.New()
	g := e.Group("/api/v0")
	RegisterRoutes(g, (*bun.DB)(nil), apps, apps, grants, grants)
	return e
}

func do(e *echo.Echo, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// -------- tests --------

func TestListApplications_OK(t *testing.T) {
	apps := &fakeApps{list: []*ConsentedApplication{
		{ID: uuid.New(), Source: "s", ClientID: "abc", Origin: OriginThirdParty, ResolutionConfidence: ConfidenceUnresolved},
	}}
	rec := do(newTestServer(apps, &fakeGrants{}), "/api/v0/consented-applications?origin=third_party")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got AppListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 {
		t.Fatalf("want 1 item, got %+v", got)
	}
	if apps.lastFilter.Origin != "third_party" {
		t.Errorf("origin filter not forwarded: %q", apps.lastFilter.Origin)
	}
}

func TestGetApplication_NotFound(t *testing.T) {
	rec := do(newTestServer(&fakeApps{byID: map[uuid.UUID]*ConsentedApplication{}}, &fakeGrants{}),
		"/api/v0/consented-applications/"+uuid.New().String())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetApplication_BadID(t *testing.T) {
	rec := do(newTestServer(&fakeApps{}, &fakeGrants{}), "/api/v0/consented-applications/not-a-uuid")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListGrants_FilterByApplication(t *testing.T) {
	appID := uuid.New()
	grants := &fakeGrants{list: []*ConsentGrant{
		{ID: uuid.New(), Source: "s", ExternalID: "g1", ConsentedApplicationID: appID, GrantType: GrantDelegated},
	}}
	rec := do(newTestServer(&fakeApps{}, grants),
		"/api/v0/consent-grants?consented_application_id="+appID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if grants.lastFilter.ConsentedApplicationID == nil || *grants.lastFilter.ConsentedApplicationID != appID {
		t.Errorf("consented_application_id filter not forwarded: %v", grants.lastFilter.ConsentedApplicationID)
	}
}

func TestGetGrant_NotFound(t *testing.T) {
	rec := do(newTestServer(&fakeApps{}, &fakeGrants{byID: map[uuid.UUID]*ConsentGrant{}}),
		"/api/v0/consent-grants/"+uuid.New().String())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

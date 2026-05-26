// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_profile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func newTestServer(repo Repository) *echo.Echo {
	e := echo.New()
	g := e.Group("/api/v0")
	RegisterRoutes(g, NewService(repo))
	return e
}

func TestProfileRouteOK(t *testing.T) {
	e := newTestServer(richFixture())
	req := httptest.NewRequest(http.MethodGet, "/api/v0/persons/"+uuidStr()+"/access-profile", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got AccessProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FullName != "Marcus Vale" || len(got.Applications) != 2 {
		t.Fatalf("unexpected profile: %+v", got)
	}
}

func TestProfileRouteNotFound(t *testing.T) {
	e := newTestServer(&fakeRepo{personErr: ErrPersonNotFound})
	req := httptest.NewRequest(http.MethodGet, "/api/v0/persons/"+uuidStr()+"/access-profile", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestProfileRouteBadID(t *testing.T) {
	e := newTestServer(richFixture())
	req := httptest.NewRequest(http.MethodGet, "/api/v0/persons/not-a-uuid/access-profile", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func uuidStr() string { return "11111111-1111-1111-1111-111111111111" }

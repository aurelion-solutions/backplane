// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newTestServer(out string) (*echo.Echo, *findingFixtureIDs) {
	f := newFinding()
	svc, _, _ := buildService(out, nil, f, nil)
	e := echo.New()
	RegisterRoutes(e.Group(""), svc)
	return e, &findingFixtureIDs{findingID: f.ID.String()}
}

type findingFixtureIDs struct{ findingID string }

func TestRouteExplainReturnsView(t *testing.T) {
	e, ids := newTestServer("Account [F] lacks MFA per [P1].")

	req := httptest.NewRequest(http.MethodPost, "/findings/"+ids.findingID+"/explanations", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	var view ExplanationView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Status != StatusCompleted || len(view.Citations) != 2 {
		t.Fatalf("view = %+v", view)
	}
}

func TestRouteExplainBadID(t *testing.T) {
	e, _ := newTestServer("x")
	req := httptest.NewRequest(http.MethodPost, "/findings/not-a-uuid/explanations", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRouteLatestNotFound(t *testing.T) {
	e, ids := newTestServer("x")
	// No explanation generated yet → latest is 404.
	req := httptest.NewRequest(http.MethodGet, "/findings/"+ids.findingID+"/explanations/latest", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRouteExplainThenLatest(t *testing.T) {
	e, ids := newTestServer("Account [F] flagged.")

	post := httptest.NewRequest(http.MethodPost, "/findings/"+ids.findingID+"/explanations", strings.NewReader(`{}`))
	post.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(httptest.NewRecorder(), post)

	get := httptest.NewRequest(http.MethodGet, "/findings/"+ids.findingID+"/explanations/latest", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, get)
	if rec.Code != http.StatusOK {
		t.Fatalf("latest status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
}

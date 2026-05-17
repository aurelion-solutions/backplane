// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func buildAppsRouter(t *testing.T) (*echo.Echo, string) {
	t.Helper()
	root := t.TempDir()
	bundle := filepath.Join(root, "popular")
	writeApp(t, bundle, "microsoft_ad", validManifest, validAccount, validDescriptor)

	e := echo.New()
	g := e.Group("")
	RegisterRoutes(g, NewFilesystemProvider(root))
	return e, root
}

func TestRoutes_ListApps(t *testing.T) {
	e, _ := buildAppsRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/cartridges/popular/apps", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got []AppListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != "microsoft_ad" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Connector != "ad" {
		t.Fatalf("connector = %q", got[0].Connector)
	}
	if got[0].StatesCount != 3 || got[0].FieldsCount != 2 {
		t.Fatalf("counts = states %d, fields %d", got[0].StatesCount, got[0].FieldsCount)
	}
}

func TestRoutes_AppDetail(t *testing.T) {
	e, _ := buildAppsRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/cartridges/popular/apps/microsoft_ad", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got AppCartridge
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Manifest.ID != "microsoft_ad" {
		t.Fatalf("id = %q", got.Manifest.ID)
	}
	if got.Account.InitialState != "not_exist" {
		t.Fatalf("initial = %q", got.Account.InitialState)
	}
	if _, ok := got.Descriptor.Fields["userPrincipalName"]; !ok {
		t.Fatalf("missing userPrincipalName: %+v", got.Descriptor.Fields)
	}
	// BasePath is a server-local path and must not surface in the
	// JSON response.
	if strings.Contains(rec.Body.String(), "BasePath") {
		t.Fatalf("BasePath leaked into response: %s", rec.Body.String())
	}
}

func TestRoutes_AppDetail_NotFound(t *testing.T) {
	e, _ := buildAppsRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/cartridges/popular/apps/nonsense", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRoutes_ListApps_UnknownBundle(t *testing.T) {
	e, _ := buildAppsRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/cartridges/no-such/apps", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

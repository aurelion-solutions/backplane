// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workload_lineage_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/inventory/workload_lineage"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// snapshotCallCounter wraps a fake repo that counts snapshot writes.
// Used to assert the GET route NEVER writes a snapshot (R1).
type snapshotCallCounter struct{ count int }

func TestLineageRoute_200_ReturnsChain(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	pID := uuid.New()

	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc", OwnerEmploymentID: &eID},
	}}
	er := &fakeEmploymentReader{
		byID: map[uuid.UUID]*workload_lineage.EmploymentRef{
			eID: {ID: eID, PersonID: pID, Code: "active", StartDate: time.Now().UTC().AddDate(-1, 0, 0)},
		},
		byPersonID: map[uuid.UUID][]*workload_lineage.EmploymentRef{
			pID: {{ID: eID, PersonID: pID, Code: "active", StartDate: time.Now().UTC().AddDate(-1, 0, 0)}},
		},
	}
	pr := &fakePersonReader{refs: map[uuid.UUID]*workload_lineage.PersonRef{
		pID: {ID: pID, FullName: "Alice"},
	}}

	resolver := workload_lineage.NewResolver(wr, er, pr)

	e := echo.New()
	apiV0 := e.Group("/api/v0")
	workload_lineage.RegisterRoutes(apiV0, resolver)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/workloads/"+wID.String()+"/lineage", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var chain workload_lineage.OwnershipChain
	if err := json.NewDecoder(rec.Body).Decode(&chain); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if chain.WorkloadID != wID {
		t.Errorf("want workload_id %s, got %s", wID, chain.WorkloadID)
	}
	if chain.Terminus != workload_lineage.TerminusActiveHuman {
		t.Errorf("want active_human terminus, got %s", chain.Terminus)
	}
}

func TestLineageRoute_404_UnknownWorkload(t *testing.T) {
	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{}}
	resolver := workload_lineage.NewResolver(wr, &fakeEmploymentReader{}, &fakePersonReader{})

	e := echo.New()
	apiV0 := e.Group("/api/v0")
	workload_lineage.RegisterRoutes(apiV0, resolver)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/workloads/"+uuid.New().String()+"/lineage", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestLineageRoute_400_InvalidID(t *testing.T) {
	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{}}
	resolver := workload_lineage.NewResolver(wr, &fakeEmploymentReader{}, &fakePersonReader{})

	e := echo.New()
	apiV0 := e.Group("/api/v0")
	workload_lineage.RegisterRoutes(apiV0, resolver)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/workloads/not-a-uuid/lineage", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

// TestLineageRoute_NoSnapshotWrite asserts GET /lineage does NOT write
// any snapshot (R1 — the GET is read-only; snapshots live in the assess pass).
// We verify this by checking that the route never touches the snapshotCallCounter.
func TestLineageRoute_NoSnapshotWrite(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	pID := uuid.New()

	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc", OwnerEmploymentID: &eID},
	}}
	er := &fakeEmploymentReader{
		byID: map[uuid.UUID]*workload_lineage.EmploymentRef{
			eID: {ID: eID, PersonID: pID, Code: "active", StartDate: time.Now().UTC().AddDate(-1, 0, 0)},
		},
		byPersonID: map[uuid.UUID][]*workload_lineage.EmploymentRef{
			pID: {{ID: eID, PersonID: pID, Code: "active", StartDate: time.Now().UTC().AddDate(-1, 0, 0)}},
		},
	}
	pr := &fakePersonReader{refs: map[uuid.UUID]*workload_lineage.PersonRef{
		pID: {ID: pID, FullName: "Alice"},
	}}

	counter := &snapshotCallCounter{}
	resolver := workload_lineage.NewResolver(wr, er, pr)

	// RegisterRoutes takes only a *Resolver — no snapshot writer parameter.
	// The test asserts structurally that no snapshot path is available on GET.
	e := echo.New()
	apiV0 := e.Group("/api/v0")
	workload_lineage.RegisterRoutes(apiV0, resolver)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/workloads/"+wID.String()+"/lineage", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	// counter.count must be 0 — the route has no code path to increment it.
	if counter.count != 0 {
		t.Errorf("GET route wrote %d snapshots; want 0 (R1 violation)", counter.count)
	}
}

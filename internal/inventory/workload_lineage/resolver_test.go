// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workload_lineage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/inventory/workload_lineage"
	"github.com/google/uuid"
)

// ---- fake readers ----

type fakeWorkloadReader struct {
	refs map[uuid.UUID]*workload_lineage.WorkloadRef
}

func (f *fakeWorkloadReader) GetByID(_ context.Context, id uuid.UUID) (*workload_lineage.WorkloadRef, error) {
	if r, ok := f.refs[id]; ok {
		return r, nil
	}
	return nil, workload_lineage.ErrReaderNotFound
}

type fakeEmploymentReader struct {
	byID       map[uuid.UUID]*workload_lineage.EmploymentRef
	byPersonID map[uuid.UUID][]*workload_lineage.EmploymentRef
}

func (f *fakeEmploymentReader) GetByID(_ context.Context, id uuid.UUID) (*workload_lineage.EmploymentRef, error) {
	if r, ok := f.byID[id]; ok {
		return r, nil
	}
	return nil, workload_lineage.ErrReaderNotFound
}

func (f *fakeEmploymentReader) ListByPerson(_ context.Context, personID uuid.UUID) ([]*workload_lineage.EmploymentRef, error) {
	return f.byPersonID[personID], nil
}

type fakePersonReader struct {
	refs map[uuid.UUID]*workload_lineage.PersonRef
}

func (f *fakePersonReader) GetByID(_ context.Context, id uuid.UUID) (*workload_lineage.PersonRef, error) {
	if r, ok := f.refs[id]; ok {
		return r, nil
	}
	return nil, workload_lineage.ErrReaderNotFound
}

// ---- helpers ----

func ptr[T any](v T) *T { return &v }

func dayAgo() time.Time  { return time.Now().UTC().AddDate(0, 0, -1) }
func yearAgo() time.Time { return time.Now().UTC().AddDate(-1, 0, 0) }
func yesterday() time.Time {
	// EndDate exclusive: set to today so IsActiveAt(now) is false.
	return time.Now().UTC().Truncate(24 * time.Hour)
}

// ---- tests ----

func TestResolve_ActiveOwner(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	pID := uuid.New()

	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc-a", OwnerEmploymentID: &eID},
	}}
	er := &fakeEmploymentReader{
		byID: map[uuid.UUID]*workload_lineage.EmploymentRef{
			eID: {ID: eID, PersonID: pID, Code: "active", StartDate: yearAgo()},
		},
		byPersonID: map[uuid.UUID][]*workload_lineage.EmploymentRef{
			pID: {{ID: eID, PersonID: pID, Code: "active", StartDate: yearAgo()}},
		},
	}
	pr := &fakePersonReader{refs: map[uuid.UUID]*workload_lineage.PersonRef{
		pID: {ID: pID, FullName: "Alice"},
	}}

	r := workload_lineage.NewResolver(wr, er, pr)
	chain, err := r.Resolve(context.Background(), wID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Terminus != workload_lineage.TerminusActiveHuman {
		t.Errorf("want active_human, got %s", chain.Terminus)
	}
	if len(chain.Links) != 3 {
		t.Errorf("want 3 links, got %d", len(chain.Links))
	}
}

func TestResolve_TerminatedOwner_AllEmploymentsEnded(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	pID := uuid.New()
	ended := yesterday()

	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc-b", OwnerEmploymentID: &eID},
	}}
	er := &fakeEmploymentReader{
		byID: map[uuid.UUID]*workload_lineage.EmploymentRef{
			eID: {ID: eID, PersonID: pID, Code: "terminated", StartDate: yearAgo(), EndDate: &ended},
		},
		byPersonID: map[uuid.UUID][]*workload_lineage.EmploymentRef{
			pID: {{ID: eID, PersonID: pID, Code: "terminated", StartDate: yearAgo(), EndDate: &ended}},
		},
	}
	pr := &fakePersonReader{refs: map[uuid.UUID]*workload_lineage.PersonRef{
		pID: {ID: pID, FullName: "Bob"},
	}}

	r := workload_lineage.NewResolver(wr, er, pr)
	chain, err := r.Resolve(context.Background(), wID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Terminus != workload_lineage.TerminusTerminatedHuman {
		t.Errorf("want terminated_human, got %s", chain.Terminus)
	}
	// Person link end_date should be the latest termination date.
	personLink := chain.Links[len(chain.Links)-1]
	if personLink.EndDate == nil {
		t.Error("want person link end_date set, got nil")
	}
}

func TestResolve_MixedEmployments_ActiveHuman(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	e2ID := uuid.New()
	pID := uuid.New()
	ended := yesterday()

	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc-c", OwnerEmploymentID: &eID},
	}}
	er := &fakeEmploymentReader{
		byID: map[uuid.UUID]*workload_lineage.EmploymentRef{
			eID: {ID: eID, PersonID: pID, Code: "active", StartDate: yearAgo()},
		},
		byPersonID: map[uuid.UUID][]*workload_lineage.EmploymentRef{
			pID: {
				{ID: eID, PersonID: pID, Code: "active", StartDate: yearAgo()},
				{ID: e2ID, PersonID: pID, Code: "old", StartDate: yearAgo().AddDate(-1, 0, 0), EndDate: &ended},
			},
		},
	}
	pr := &fakePersonReader{refs: map[uuid.UUID]*workload_lineage.PersonRef{
		pID: {ID: pID, FullName: "Carol"},
	}}

	r := workload_lineage.NewResolver(wr, er, pr)
	chain, err := r.Resolve(context.Background(), wID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Terminus != workload_lineage.TerminusActiveHuman {
		t.Errorf("mixed employments (one active): want active_human, got %s", chain.Terminus)
	}
}

func TestResolve_NullOwnerEmployment_Unowned(t *testing.T) {
	wID := uuid.New()
	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc-d", OwnerEmploymentID: nil},
	}}
	r := workload_lineage.NewResolver(wr, &fakeEmploymentReader{}, &fakePersonReader{})
	chain, err := r.Resolve(context.Background(), wID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Terminus != workload_lineage.TerminusUnowned {
		t.Errorf("want unowned, got %s", chain.Terminus)
	}
}

func TestResolve_OwningEmploymentMissing_BrokenLink(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc-e", OwnerEmploymentID: &eID},
	}}
	// eID not in byID → not found
	er := &fakeEmploymentReader{byID: map[uuid.UUID]*workload_lineage.EmploymentRef{}}
	r := workload_lineage.NewResolver(wr, er, &fakePersonReader{})
	chain, err := r.Resolve(context.Background(), wID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Terminus != workload_lineage.TerminusBrokenLink {
		t.Errorf("want broken_link, got %s", chain.Terminus)
	}
}

func TestResolve_PersonMissing_BrokenLink(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	pID := uuid.New()
	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc-f", OwnerEmploymentID: &eID},
	}}
	er := &fakeEmploymentReader{
		byID: map[uuid.UUID]*workload_lineage.EmploymentRef{
			eID: {ID: eID, PersonID: pID, Code: "active", StartDate: yearAgo()},
		},
		byPersonID: map[uuid.UUID][]*workload_lineage.EmploymentRef{},
	}
	// pID not in persons → not found
	pr := &fakePersonReader{refs: map[uuid.UUID]*workload_lineage.PersonRef{}}
	r := workload_lineage.NewResolver(wr, er, pr)
	chain, err := r.Resolve(context.Background(), wID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Terminus != workload_lineage.TerminusBrokenLink {
		t.Errorf("want broken_link, got %s", chain.Terminus)
	}
}

func TestResolve_UnknownWorkload_ErrWorkloadNotFound(t *testing.T) {
	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{}}
	r := workload_lineage.NewResolver(wr, &fakeEmploymentReader{}, &fakePersonReader{})
	_, err := r.Resolve(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected ErrWorkloadNotFound, got nil")
	}
	if err != workload_lineage.ErrWorkloadNotFound {
		t.Errorf("want ErrWorkloadNotFound, got %v", err)
	}
}

func TestChainHash_StableAcrossIdenticalResolutions(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	pID := uuid.New()
	ended := yesterday()

	makeChain := func() workload_lineage.OwnershipChain {
		return workload_lineage.OwnershipChain{
			WorkloadID: wID,
			Terminus:   workload_lineage.TerminusTerminatedHuman,
			ResolvedAt: time.Now().UTC(), // wall-clock differs — must NOT affect hash
			Links: []workload_lineage.ChainLink{
				{Kind: "workload", RefID: wID.String(), Label: "svc"},
				{Kind: "employment", RefID: eID.String(), Terminated: true, EndDate: &ended},
				{Kind: "person", RefID: pID.String(), Label: "Dave", Terminated: true, EndDate: &ended},
			},
		}
	}

	h1 := makeChain().ChainHash()
	h2 := makeChain().ChainHash()
	if h1 != h2 {
		t.Errorf("identical chains produced different hashes: %s vs %s", h1, h2)
	}
}

func TestChainHash_DiffersWhenEndDateChanges(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	t1 := yesterday()
	t2 := yesterday().Add(-24 * time.Hour)

	makeChain := func(end time.Time) workload_lineage.OwnershipChain {
		return workload_lineage.OwnershipChain{
			WorkloadID: wID,
			Terminus:   workload_lineage.TerminusTerminatedHuman,
			Links: []workload_lineage.ChainLink{
				{Kind: "workload", RefID: wID.String()},
				{Kind: "employment", RefID: eID.String(), Terminated: true, EndDate: &end},
			},
		}
	}

	h1 := makeChain(t1).ChainHash()
	h2 := makeChain(t2).ChainHash()
	if h1 == h2 {
		t.Error("chains with different end_dates should produce different hashes")
	}
}

// TestResolve_AllEmploymentsEnded verifies that the person link carries
// the latest termination date when all employments have ended.
func TestResolve_AllEmploymentsEnded_PersonEndDate(t *testing.T) {
	wID := uuid.New()
	eID1 := uuid.New()
	eID2 := uuid.New()
	pID := uuid.New()

	earlier := yesterday().Add(-48 * time.Hour)
	later := yesterday()

	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc", OwnerEmploymentID: &eID1},
	}}
	er := &fakeEmploymentReader{
		byID: map[uuid.UUID]*workload_lineage.EmploymentRef{
			eID1: {ID: eID1, PersonID: pID, Code: "t1", StartDate: yearAgo(), EndDate: &earlier},
		},
		byPersonID: map[uuid.UUID][]*workload_lineage.EmploymentRef{
			pID: {
				{ID: eID1, PersonID: pID, Code: "t1", StartDate: yearAgo(), EndDate: &earlier},
				{ID: eID2, PersonID: pID, Code: "t2", StartDate: yearAgo(), EndDate: &later},
			},
		},
	}
	pr := &fakePersonReader{refs: map[uuid.UUID]*workload_lineage.PersonRef{
		pID: {ID: pID, FullName: "Eve"},
	}}

	r := workload_lineage.NewResolver(wr, er, pr)
	chain, err := r.Resolve(context.Background(), wID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Terminus != workload_lineage.TerminusTerminatedHuman {
		t.Fatalf("want terminated_human, got %s", chain.Terminus)
	}
	personLink := chain.Links[len(chain.Links)-1]
	if personLink.EndDate == nil {
		t.Fatal("want person link end_date, got nil")
	}
	// Must be the later of the two end dates.
	if !personLink.EndDate.Equal(later) {
		t.Errorf("want end_date %v, got %v", later, *personLink.EndDate)
	}
}

// TestResolve_PersonLabel confirms the person link carries FullName as Label.
func TestResolve_PersonLabel(t *testing.T) {
	wID := uuid.New()
	eID := uuid.New()
	pID := uuid.New()

	wr := &fakeWorkloadReader{refs: map[uuid.UUID]*workload_lineage.WorkloadRef{
		wID: {ID: wID, Name: "svc", OwnerEmploymentID: &eID},
	}}
	er := &fakeEmploymentReader{
		byID: map[uuid.UUID]*workload_lineage.EmploymentRef{
			eID: {ID: eID, PersonID: pID, Code: "active", StartDate: yearAgo()},
		},
		byPersonID: map[uuid.UUID][]*workload_lineage.EmploymentRef{
			pID: {{ID: eID, PersonID: pID, Code: "active", StartDate: yearAgo()}},
		},
	}
	pr := &fakePersonReader{refs: map[uuid.UUID]*workload_lineage.PersonRef{
		pID: {ID: pID, FullName: "Frank Sinatra"},
	}}

	r := workload_lineage.NewResolver(wr, er, pr)
	chain, err := r.Resolve(context.Background(), wID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	personLink := chain.Links[len(chain.Links)-1]
	if personLink.Label != "Frank Sinatra" {
		t.Errorf("want label %q, got %q", "Frank Sinatra", personLink.Label)
	}
}

// Ensure the fmt package is used (avoid import error on linters).
var _ = fmt.Sprintf

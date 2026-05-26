// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package evidence_chain

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeRepo is an in-memory append-only store keyed by chain_hash.
type fakeRepo struct {
	byHash map[string]*EvidenceChain
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byHash: map[string]*EvidenceChain{}} }

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*EvidenceChain, error) {
	for _, c := range f.byHash {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) GetByChainHash(_ context.Context, hash string) (*EvidenceChain, error) {
	if c, ok := f.byHash[hash]; ok {
		return c, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) Insert(_ context.Context, c *EvidenceChain) (bool, error) {
	if _, ok := f.byHash[c.ChainHash]; ok {
		return false, nil
	}
	f.byHash[c.ChainHash] = c
	return true, nil
}

func params() RecordParams {
	grant := uuid.New()
	return RecordParams{
		ScanRunID:         uuid.New(),
		CapabilityGrantID: &grant,
		PolicyRef:         "ispm-core-identity-posture/terminated_subject_access",
	}
}

func TestChainHash_Deterministic(t *testing.T) {
	p := params()
	if ChainHash(p) != ChainHash(p) {
		t.Fatal("same params must yield same hash")
	}
	p2 := p
	other := uuid.New()
	p2.CapabilityGrantID = &other
	if ChainHash(p) == ChainHash(p2) {
		t.Fatal("different lineage must yield different hash")
	}
}

func TestRecordChain_Idempotent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	p := params()

	first, err := svc.RecordChain(context.Background(), p)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.RecordChain(context.Background(), p)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("re-record must return the same row: %s vs %s", first.ID, second.ID)
	}
	if len(repo.byHash) != 1 {
		t.Fatalf("append-only idempotent: want 1 stored row, got %d", len(repo.byHash))
	}
}

func TestRecordChain_SetsHashAndRun(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	p := params()
	got, err := svc.RecordChain(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ChainHash == "" {
		t.Fatal("chain_hash must be set")
	}
	if got.ScanRunID != p.ScanRunID {
		t.Fatal("scan_run anchor must be preserved")
	}
}

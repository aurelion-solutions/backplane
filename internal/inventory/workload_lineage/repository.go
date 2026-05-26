// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workload_lineage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for lineage snapshots.
// Append-only — no Update, no Delete (projection-table discipline).
type Repository interface {
	Insert(ctx context.Context, snap *WorkloadLineageSnapshot) error
	ListByWorkload(ctx context.Context, workloadID uuid.UUID) ([]*WorkloadLineageSnapshot, error)
}

// BunRepository is the Postgres-backed implementation.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// Insert persists a snapshot row. ON CONFLICT DO NOTHING makes it
// idempotent: re-resolving an unchanged chain (same chain_hash) is a
// no-op — the existing row is kept and no error is returned.
func (r *BunRepository) Insert(ctx context.Context, snap *WorkloadLineageSnapshot) error {
	_, err := r.db.NewInsert().
		Model(snap).
		On("CONFLICT (workload_id, chain_hash) DO NOTHING").
		Exec(ctx)
	return err
}

// ListByWorkload returns all snapshots for a workload, newest first.
func (r *BunRepository) ListByWorkload(ctx context.Context, workloadID uuid.UUID) ([]*WorkloadLineageSnapshot, error) {
	out := []*WorkloadLineageSnapshot{}
	err := r.db.NewSelect().Model(&out).
		Where("workload_id = ?", workloadID).
		Order("resolved_at DESC").
		Scan(ctx)
	return out, err
}

// RecordSnapshot resolves the chain into a snapshot row and persists it
// idempotently. Invoked only from the assess pass — never from GET.
func (r *BunRepository) RecordSnapshot(ctx context.Context, chain OwnershipChain) error {
	now := time.Now().UTC()
	snap := &WorkloadLineageSnapshot{
		ID:         uuid.New(),
		WorkloadID: chain.WorkloadID,
		ResolvedAt: chain.ResolvedAt,
		Terminus:   chain.Terminus,
		Chain:      chain,
		ChainHash:  chain.ChainHash(),
		CreatedAt:  now,
	}
	return r.Insert(ctx, snap)
}

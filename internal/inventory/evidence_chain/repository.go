// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package evidence_chain

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for EvidenceChain rows.
//
// Insert is append-only with conflict-ignore on chain_hash: it reports
// whether a new row was written. GetByChainHash lets the service return
// the canonical row after an idempotent no-op insert.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*EvidenceChain, error)
	GetByChainHash(ctx context.Context, hash string) (*EvidenceChain, error)
	ListByFinding(ctx context.Context, findingID uuid.UUID) ([]*EvidenceChain, error)
	Insert(ctx context.Context, c *EvidenceChain) (inserted bool, err error)
}

// BunRepository is the production Postgres-backed Repository.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// GetByID returns one row by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*EvidenceChain, error) {
	row := new(EvidenceChain)
	err := r.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return row, nil
}

// GetByChainHash returns one row by its deterministic chain hash.
func (r *BunRepository) GetByChainHash(ctx context.Context, hash string) (*EvidenceChain, error) {
	row := new(EvidenceChain)
	err := r.db.NewSelect().Model(row).Where("chain_hash = ?", hash).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return row, nil
}

// ListByFinding returns every chain row anchored to one finding,
// oldest first.
func (r *BunRepository) ListByFinding(ctx context.Context, findingID uuid.UUID) ([]*EvidenceChain, error) {
	var rows []*EvidenceChain
	err := r.db.NewSelect().
		Model(&rows).
		Where("finding_id = ?", findingID).
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Insert appends a chain row, ignoring the conflict when chain_hash
// already exists (append-only, never mutating). Returns inserted=false
// when the row was already present.
func (r *BunRepository) Insert(ctx context.Context, row *EvidenceChain) (bool, error) {
	res, err := r.db.NewInsert().
		Model(row).
		On("CONFLICT (chain_hash) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

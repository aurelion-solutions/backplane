// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employment_record_matches

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ErrNotFound signals that no match exists for the supplied key.
var ErrNotFound = errors.New("employment_record_matches: not found")

// Repository is the persistence boundary for EmploymentRecordMatch.
//
// Idempotency key is (source, source_record_external_id,
// period_start_date) — call GetByKey before Insert. The unique index
// will reject duplicates otherwise.
type Repository interface {
	GetByKey(ctx context.Context, db bun.IDB, source, externalID string, periodStartDate time.Time) (*EmploymentRecordMatch, error)
	Insert(ctx context.Context, db bun.IDB, m *EmploymentRecordMatch) error
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct{}

// NewBunRepository constructs a BunRepository.
func NewBunRepository() *BunRepository {
	return &BunRepository{}
}

// GetByKey returns the match keyed by
// (source, external_id, period_start_date) or ErrNotFound.
func (r *BunRepository) GetByKey(ctx context.Context, db bun.IDB, source, externalID string, periodStartDate time.Time) (*EmploymentRecordMatch, error) {
	m := new(EmploymentRecordMatch)
	err := db.NewSelect().
		Model(m).
		Where("source = ?", source).
		Where("source_record_external_id = ?", externalID).
		Where("period_start_date = ?", periodStartDate.Format("2006-01-02")).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

// Insert creates a new match row. Callers should ensure idempotency
// by calling GetByKey first.
func (r *BunRepository) Insert(ctx context.Context, db bun.IDB, m *EmploymentRecordMatch) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	_, err := db.NewInsert().Model(m).Exec(ctx)
	return err
}

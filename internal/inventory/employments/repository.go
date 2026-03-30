// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employments

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// PersonResolver lifts a person external_id into a UUID. Wired via a
// persons-slice adapter at the composition root.
type PersonResolver interface {
	PersonIDByExternalID(ctx context.Context, externalID string) (uuid.UUID, bool, error)
}

// OrgUnitResolver lifts an org_unit external_id into a UUID.
type OrgUnitResolver interface {
	OrgUnitIDByExternalID(ctx context.Context, externalID string) (uuid.UUID, bool, error)
}

// Repository is the persistence boundary for Employment aggregates.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Employment, error)
	ListByPerson(ctx context.Context, personID uuid.UUID) ([]*Employment, error)
	ListActiveByPerson(ctx context.Context, personID uuid.UUID, at time.Time) ([]*Employment, error)
	List(ctx context.Context, limit, offset int) ([]*Employment, int, error)
	Insert(ctx context.Context, e *Employment) error
	Update(ctx context.Context, e *Employment) error

	ListAttributes(ctx context.Context, employmentID uuid.UUID) ([]*EmploymentAttribute, error)
	GetAttribute(ctx context.Context, employmentID uuid.UUID, key string) (*EmploymentAttribute, error)
	UpsertAttribute(ctx context.Context, a *EmploymentAttribute) error
	DeleteAttribute(ctx context.Context, employmentID uuid.UUID, key string) error

	BulkUpsert(
		ctx context.Context,
		items []BulkItem,
		persons PersonResolver,
		orgUnits OrgUnitResolver,
		idGen func() uuid.UUID,
	) (int, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// GetByID returns one row by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Employment, error) {
	e := new(Employment)
	err := r.db.NewSelect().Model(e).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

// ListByPerson returns every employment owned by a person, sorted by
// start_date ASC.
func (r *BunRepository) ListByPerson(ctx context.Context, personID uuid.UUID) ([]*Employment, error) {
	out := []*Employment{}
	err := r.db.NewSelect().Model(&out).
		Where("person_id = ?", personID).
		Order("start_date ASC").
		Scan(ctx)
	return out, err
}

// ListActiveByPerson returns every employment of a person that is
// active on `at` (start_date ≤ at AND (end_date IS NULL OR end_date > at)).
func (r *BunRepository) ListActiveByPerson(ctx context.Context, personID uuid.UUID, at time.Time) ([]*Employment, error) {
	out := []*Employment{}
	err := r.db.NewSelect().Model(&out).
		Where("person_id = ?", personID).
		Where("start_date <= ?", at).
		Where("(end_date IS NULL OR end_date > ?)", at).
		Order("start_date ASC").
		Scan(ctx)
	return out, err
}

// List returns a paginated slice + total row count.
func (r *BunRepository) List(ctx context.Context, limit, offset int) ([]*Employment, int, error) {
	out := []*Employment{}
	q := r.db.NewSelect().Model(&out).Order("start_date DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	total, err := q.ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Insert persists a new Employment.
func (r *BunRepository) Insert(ctx context.Context, e *Employment) error {
	_, err := r.db.NewInsert().Model(e).Exec(ctx)
	return err
}

// Update writes back mutable columns.
func (r *BunRepository) Update(ctx context.Context, e *Employment) error {
	_, err := r.db.NewUpdate().
		Model(e).
		Column("code", "start_date", "end_date", "org_unit_id", "description", "updated_at").
		Where("id = ?", e.ID).
		Exec(ctx)
	return err
}

// ListAttributes returns every attribute of an employment.
func (r *BunRepository) ListAttributes(ctx context.Context, employmentID uuid.UUID) ([]*EmploymentAttribute, error) {
	out := []*EmploymentAttribute{}
	err := r.db.NewSelect().Model(&out).
		Where("employment_id = ?", employmentID).
		Order("key ASC").Scan(ctx)
	return out, err
}

// GetAttribute returns one (employment, key) attribute.
func (r *BunRepository) GetAttribute(ctx context.Context, employmentID uuid.UUID, key string) (*EmploymentAttribute, error) {
	a := new(EmploymentAttribute)
	err := r.db.NewSelect().Model(a).
		Where("employment_id = ?", employmentID).
		Where("key = ?", key).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAttributeNotFound
		}
		return nil, err
	}
	return a, nil
}

// UpsertAttribute inserts or updates the (employment, key) row.
func (r *BunRepository) UpsertAttribute(ctx context.Context, a *EmploymentAttribute) error {
	_, err := r.db.NewInsert().
		Model(a).
		On("CONFLICT (employment_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

// DeleteAttribute removes one (employment, key) row.
func (r *BunRepository) DeleteAttribute(ctx context.Context, employmentID uuid.UUID, key string) error {
	res, err := r.db.NewDelete().
		Model((*EmploymentAttribute)(nil)).
		Where("employment_id = ?", employmentID).
		Where("key = ?", key).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrAttributeNotFound
	}
	return nil
}

// BulkUpsert reconciles a batch of employments in one transaction.
//
// Bulk semantics: each (person_external_id, code, start_date) tuple
// identifies an employment for upsert purposes. Same person + same
// code + different start_date = a different employment row. No
// closed-state-machine — we trust the caller's intent.
func (r *BunRepository) BulkUpsert(
	ctx context.Context,
	items []BulkItem,
	persons PersonResolver,
	orgUnits OrgUnitResolver,
	idGen func() uuid.UUID,
) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, it := range items {
		personID, ok, err := persons.PersonIDByExternalID(ctx, it.PersonExternalID)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, ErrPersonNotFound
		}
		var orgUnitID *uuid.UUID
		if it.OrgUnitExternalID != nil {
			ouID, ok, err := orgUnits.OrgUnitIDByExternalID(ctx, *it.OrgUnitExternalID)
			if err != nil {
				return 0, err
			}
			if !ok {
				return 0, ErrOrgUnitNotFound
			}
			orgUnitID = &ouID
		}
		// Find an existing row with the same (person, code, start_date).
		existing := new(Employment)
		err = tx.NewSelect().Model(existing).
			Where("person_id = ?", personID).
			Where("code = ?", it.Code).
			Where("start_date = ?", it.StartDate).
			Scan(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if errors.Is(err, sql.ErrNoRows) {
			existing = &Employment{
				ID:          idGen(),
				PersonID:    personID,
				Code:        it.Code,
				StartDate:   it.StartDate,
				EndDate:     it.EndDate,
				OrgUnitID:   orgUnitID,
				Description: it.Description,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if _, err := tx.NewInsert().Model(existing).Exec(ctx); err != nil {
				return 0, err
			}
		} else {
			existing.EndDate = it.EndDate
			existing.OrgUnitID = orgUnitID
			existing.Description = it.Description
			existing.UpdatedAt = now
			if _, err := tx.NewUpdate().Model(existing).
				Column("end_date", "org_unit_id", "description", "updated_at").
				Where("id = ?", existing.ID).
				Exec(ctx); err != nil {
				return 0, err
			}
		}

		for k, v := range it.Attributes {
			a := &EmploymentAttribute{
				ID:           idGen(),
				EmploymentID: existing.ID,
				Key:          k,
				Value:        v,
			}
			if _, err := tx.NewInsert().Model(a).
				On("CONFLICT (employment_id, key) DO UPDATE").
				Set("value = EXCLUDED.value").
				Exec(ctx); err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(items), nil
}

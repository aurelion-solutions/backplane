// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package persons

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Person aggregates.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Person, error)
	GetByExternalID(ctx context.Context, externalID string) (*Person, error)
	List(ctx context.Context, limit, offset int) ([]*Person, int, error)
	Insert(ctx context.Context, p *Person) error

	ListAttributes(ctx context.Context, personID uuid.UUID) ([]*PersonAttribute, error)
	GetAttribute(ctx context.Context, personID uuid.UUID, key string) (*PersonAttribute, error)
	UpsertAttribute(ctx context.Context, a *PersonAttribute) error
	DeleteAttribute(ctx context.Context, personID uuid.UUID, key string) error

	// BulkUpsert inserts new Persons keyed by external_id and updates
	// existing ones, plus reconciles the supplied attribute map per
	// person (upsert by key). Returns the count of input items
	// processed. Wraps the write in a single transaction.
	BulkUpsert(ctx context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository bound to db.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// GetByID returns one Person by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Person, error) {
	p := new(Person)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

// GetByExternalID returns one Person by external_id, or ErrNotFound.
func (r *BunRepository) GetByExternalID(ctx context.Context, externalID string) (*Person, error) {
	p := new(Person)
	err := r.db.NewSelect().Model(p).Where("external_id = ?", externalID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

// List returns a paginated slice plus the total row count.
func (r *BunRepository) List(ctx context.Context, limit, offset int) ([]*Person, int, error) {
	out := []*Person{}
	q := r.db.NewSelect().Model(&out).Order("updated_at DESC", "id ASC")
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

// Insert persists a new Person. Caller stamps the UUID.
func (r *BunRepository) Insert(ctx context.Context, p *Person) error {
	_, err := r.db.NewInsert().Model(p).Exec(ctx)
	return err
}

// ListAttributes returns every attribute for a person, ordered by key.
func (r *BunRepository) ListAttributes(ctx context.Context, personID uuid.UUID) ([]*PersonAttribute, error) {
	out := []*PersonAttribute{}
	err := r.db.NewSelect().Model(&out).Where("person_id = ?", personID).Order("key ASC").Scan(ctx)
	return out, err
}

// GetAttribute returns one (person, key) attribute or ErrNotFound.
func (r *BunRepository) GetAttribute(ctx context.Context, personID uuid.UUID, key string) (*PersonAttribute, error) {
	a := new(PersonAttribute)
	err := r.db.NewSelect().Model(a).
		Where("person_id = ?", personID).
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

// UpsertAttribute inserts or updates the (person, key) row.
func (r *BunRepository) UpsertAttribute(ctx context.Context, a *PersonAttribute) error {
	_, err := r.db.NewInsert().
		Model(a).
		On("CONFLICT (person_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

// DeleteAttribute removes one (person, key) row.
func (r *BunRepository) DeleteAttribute(ctx context.Context, personID uuid.UUID, key string) error {
	res, err := r.db.NewDelete().
		Model((*PersonAttribute)(nil)).
		Where("person_id = ?", personID).
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

// BulkUpsert reconciles a batch of (external_id, full_name, attributes)
// records in one transaction. Insert-or-update on persons, then upsert
// every supplied (person, key) attribute.
func (r *BunRepository) BulkUpsert(ctx context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	for _, it := range items {
		p := &Person{
			ID:         idGen(),
			ExternalID: it.ExternalID,
			FullName:   it.FullName,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		// Insert-or-update on (external_id); RETURNING id picks up the
		// id of an existing row when the constraint fires.
		_, err := tx.NewInsert().Model(p).
			On("CONFLICT (external_id) DO UPDATE").
			Set("full_name  = EXCLUDED.full_name").
			Set("updated_at = EXCLUDED.updated_at").
			Returning("id").
			Exec(ctx)
		if err != nil {
			return 0, err
		}
		for k, v := range it.Attributes {
			attr := &PersonAttribute{
				ID:       idGen(),
				PersonID: p.ID,
				Key:      k,
				Value:    v,
			}
			_, err := tx.NewInsert().Model(attr).
				On("CONFLICT (person_id, key) DO UPDATE").
				Set("value = EXCLUDED.value").
				Exec(ctx)
			if err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(items), nil
}

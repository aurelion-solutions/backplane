// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package customers

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Customer aggregates.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Customer, error)
	GetByExternalID(ctx context.Context, externalID string) (*Customer, error)
	List(ctx context.Context, limit, offset int) ([]*Customer, int, error)
	Insert(ctx context.Context, c *Customer) error
	Update(ctx context.Context, c *Customer) error

	ListAttributes(ctx context.Context, customerID uuid.UUID) ([]*CustomerAttribute, error)
	GetAttribute(ctx context.Context, customerID uuid.UUID, key string) (*CustomerAttribute, error)
	UpsertAttribute(ctx context.Context, a *CustomerAttribute) error
	DeleteAttribute(ctx context.Context, customerID uuid.UUID, key string) error

	BulkUpsert(ctx context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct {
	db  *bun.DB
	now func() time.Time
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB, now func() time.Time) *BunRepository {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &BunRepository{db: db, now: now}
}

// GetByID returns one row by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Customer, error) {
	c := new(Customer)
	err := r.db.NewSelect().Model(c).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return c, nil
}

// GetByExternalID returns one row by external_id.
func (r *BunRepository) GetByExternalID(ctx context.Context, externalID string) (*Customer, error) {
	c := new(Customer)
	err := r.db.NewSelect().Model(c).Where("external_id = ?", externalID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return c, nil
}

// List returns a paginated slice + total row count.
func (r *BunRepository) List(ctx context.Context, limit, offset int) ([]*Customer, int, error) {
	out := []*Customer{}
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

// Insert persists a new Customer.
func (r *BunRepository) Insert(ctx context.Context, c *Customer) error {
	_, err := r.db.NewInsert().Model(c).Exec(ctx)
	return err
}

// Update writes back mutable columns.
func (r *BunRepository) Update(ctx context.Context, c *Customer) error {
	_, err := r.db.NewUpdate().
		Model(c).
		Column("email_verified", "mfa_enabled", "plan_tier", "description", "updated_at").
		Where("id = ?", c.ID).
		Exec(ctx)
	return err
}

// ListAttributes returns every attribute of a customer.
func (r *BunRepository) ListAttributes(ctx context.Context, customerID uuid.UUID) ([]*CustomerAttribute, error) {
	out := []*CustomerAttribute{}
	err := r.db.NewSelect().Model(&out).Where("customer_id = ?", customerID).Order("key ASC").Scan(ctx)
	return out, err
}

// GetAttribute returns one (customer, key) attribute.
func (r *BunRepository) GetAttribute(ctx context.Context, customerID uuid.UUID, key string) (*CustomerAttribute, error) {
	a := new(CustomerAttribute)
	err := r.db.NewSelect().Model(a).
		Where("customer_id = ?", customerID).
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

// UpsertAttribute inserts or updates the (customer, key) row.
func (r *BunRepository) UpsertAttribute(ctx context.Context, a *CustomerAttribute) error {
	_, err := r.db.NewInsert().
		Model(a).
		On("CONFLICT (customer_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

// DeleteAttribute removes one (customer, key) row.
func (r *BunRepository) DeleteAttribute(ctx context.Context, customerID uuid.UUID, key string) error {
	res, err := r.db.NewDelete().
		Model((*CustomerAttribute)(nil)).
		Where("customer_id = ?", customerID).
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

// BulkUpsert reconciles a batch in one transaction.
func (r *BunRepository) BulkUpsert(ctx context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, it := range items {
		c := &Customer{
			ID:          idGen(),
			ExternalID:  it.ExternalID,
			TenantID:    it.TenantID,
			TenantRole:  it.TenantRole,
			PlanTier:    it.PlanTier,
			Description: it.Description,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		c.EmailVerified = boolDeref(it.EmailVerified, false)
		c.MFAEnabled = boolDeref(it.MFAEnabled, true)
		_, err := tx.NewInsert().Model(c).
			On("CONFLICT (external_id) DO UPDATE").
			Set("email_verified = EXCLUDED.email_verified").
			Set("tenant_id      = EXCLUDED.tenant_id").
			Set("tenant_role    = EXCLUDED.tenant_role").
			Set("plan_tier      = EXCLUDED.plan_tier").
			Set("mfa_enabled    = EXCLUDED.mfa_enabled").
			Set("description    = EXCLUDED.description").
			Set("updated_at     = EXCLUDED.updated_at").
			Returning("id").
			Exec(ctx)
		if err != nil {
			return 0, err
		}
		for k, v := range it.Attributes {
			a := &CustomerAttribute{
				ID:         idGen(),
				CustomerID: c.ID,
				Key:        k,
				Value:      v,
				CreatedAt:  now,
			}
			_, err := tx.NewInsert().Model(a).
				On("CONFLICT (customer_id, key) DO UPDATE").
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

func boolDeref(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package applications

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Application records.
// Services depend on this interface so tests can swap in an in-memory
// fake without spinning up Postgres.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Application, error)
	GetByCode(ctx context.Context, code string) (*Application, error)
	List(ctx context.Context) ([]*Application, error)
	Insert(ctx context.Context, app *Application) error
	Update(ctx context.Context, app *Application) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// BunRepository is the production implementation backed by bun + Postgres.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository bound to db.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// GetByID returns the row matching id, or ErrNotFound.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Application, error) {
	app := new(Application)
	err := r.db.NewSelect().Model(app).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return app, nil
}

// GetByCode returns the row matching the stable code, or ErrNotFound.
func (r *BunRepository) GetByCode(ctx context.Context, code string) (*Application, error) {
	app := new(Application)
	err := r.db.NewSelect().Model(app).Where("code = ?", code).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return app, nil
}

// List returns every Application ordered by name ASC. Always returns a
// non-nil slice so JSON encoders emit [] instead of null on empty
// result sets (clients pin to array shape).
func (r *BunRepository) List(ctx context.Context) ([]*Application, error) {
	out := []*Application{}
	if err := r.db.NewSelect().Model(&out).Order("name ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// Insert persists a new Application. The caller is expected to have
// stamped ID, CreatedAt, and UpdatedAt.
func (r *BunRepository) Insert(ctx context.Context, app *Application) error {
	_, err := r.db.NewInsert().Model(app).Exec(ctx)
	return err
}

// Update writes back the full mutable column set.
func (r *BunRepository) Update(ctx context.Context, app *Application) error {
	_, err := r.db.NewUpdate().
		Model(app).
		Column("name", "code", "config", "required_connector_tags", "is_active", "owner", "updated_at").
		Where("id = ?", app.ID).
		Exec(ctx)
	return err
}

// Delete removes the row by id.
func (r *BunRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.NewDelete().Model((*Application)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

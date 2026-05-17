// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package capabilities

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ErrNotFound is returned by lookup methods when no capability
// matches the supplied key.
var ErrNotFound = errors.New("capabilities: not found")

// Repository is the persistence boundary for Capability lookups.
//
// Capability rows are read-mostly catalog content — created via the
// catalog import path elsewhere. This slice only exposes read paths
// here; mutation surfaces stay where the import flow lives.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Capability, error)
	GetBySlug(ctx context.Context, slug string) (*Capability, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository { return &BunRepository{db: db} }

// GetByID returns the capability with the given id, or ErrNotFound.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Capability, error) {
	out := new(Capability)
	err := r.db.NewSelect().Model(out).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetBySlug returns the capability with the given slug, or
// ErrNotFound.
func (r *BunRepository) GetBySlug(ctx context.Context, slug string) (*Capability, error) {
	out := new(Capability)
	err := r.db.NewSelect().Model(out).Where("slug = ?", slug).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for ConnectorInstance rows.
type Repository interface {
	GetByInstanceID(ctx context.Context, instanceID string) (*ConnectorInstance, error)
	List(ctx context.Context) ([]*ConnectorInstance, error)
	ListOnline(ctx context.Context, now time.Time) ([]*ConnectorInstance, error)
	Upsert(ctx context.Context, instanceID string, tags []string, descriptor map[string]any, now time.Time) (*ConnectorInstance, error)
	DeleteStale(ctx context.Context, before time.Time) (int, error)
}

// BunRepository is the production implementation backed by bun + Postgres.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository bound to db.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// GetByInstanceID returns the row matching instanceID or ErrInstanceNotFound.
func (r *BunRepository) GetByInstanceID(ctx context.Context, instanceID string) (*ConnectorInstance, error) {
	inst := new(ConnectorInstance)
	err := r.db.NewSelect().Model(inst).Where("instance_id = ?", instanceID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInstanceNotFound
		}
		return nil, err
	}
	return inst, nil
}

// List returns every instance ordered by instance_id ASC. Always
// returns a non-nil slice so JSON encoders emit [] on empty results.
func (r *BunRepository) List(ctx context.Context) ([]*ConnectorInstance, error) {
	out := []*ConnectorInstance{}
	if err := r.db.NewSelect().Model(&out).Order("instance_id ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// ListOnline returns instances whose last_seen_at is within the online
// threshold of now. Non-nil slice on empty result.
func (r *BunRepository) ListOnline(ctx context.Context, now time.Time) ([]*ConnectorInstance, error) {
	cutoff := now.Add(-onlineThreshold)
	out := []*ConnectorInstance{}
	err := r.db.NewSelect().
		Model(&out).
		Where("last_seen_at >= ?", cutoff).
		Order("instance_id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Upsert inserts or refreshes a registration row. Tags always replace
// the stored set. Descriptor is updated only when non-nil; pass nil to
// preserve the previously-stored descriptor (heartbeat behavior).
func (r *BunRepository) Upsert(
	ctx context.Context,
	instanceID string,
	tags []string,
	descriptor map[string]any,
	now time.Time,
) (*ConnectorInstance, error) {
	if tags == nil {
		tags = []string{}
	}
	inst := &ConnectorInstance{
		ID:         uuid.New(),
		InstanceID: instanceID,
		Tags:       tags,
		LastSeenAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if descriptor != nil {
		inst.Descriptor = descriptor
	}

	q := r.db.NewInsert().Model(inst).
		On("CONFLICT (instance_id) DO UPDATE").
		Set("tags = EXCLUDED.tags").
		Set("last_seen_at = EXCLUDED.last_seen_at").
		Set("updated_at = EXCLUDED.updated_at")
	if descriptor != nil {
		q = q.Set("descriptor = EXCLUDED.descriptor")
	}
	if _, err := q.Exec(ctx); err != nil {
		return nil, err
	}

	// Re-read so the returned row reflects the merged state.
	return r.GetByInstanceID(ctx, instanceID)
}

// DeleteStale removes every instance with last_seen_at < before and
// returns the number of rows removed.
func (r *BunRepository) DeleteStale(ctx context.Context, before time.Time) (int, error) {
	res, err := r.db.NewDelete().
		Model((*ConnectorInstance)(nil)).
		Where("last_seen_at < ?", before).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

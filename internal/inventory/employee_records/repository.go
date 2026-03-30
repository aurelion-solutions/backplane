// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_records

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ApplicationResolver lifts an application code into a UUID. Wired via
// the applications-slice adapter at the composition root.
type ApplicationResolver interface {
	ApplicationIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error)
}

// Repository is the persistence boundary for the four entities owned
// by this slice (record, attribute, provider mapping, match).
type Repository interface {
	// EmployeeRecord
	GetRecordByID(ctx context.Context, id uuid.UUID) (*EmployeeRecord, error)
	GetRecordByExternal(ctx context.Context, appID uuid.UUID, externalID string) (*EmployeeRecord, error)
	ListRecords(ctx context.Context) ([]*EmployeeRecord, error)
	InsertRecord(ctx context.Context, r *EmployeeRecord) error

	// Record attributes
	ListRecordAttributes(ctx context.Context, recordID uuid.UUID) ([]*EmployeeRecordAttribute, error)
	GetRecordAttribute(ctx context.Context, recordID uuid.UUID, key string) (*EmployeeRecordAttribute, error)
	UpsertRecordAttribute(ctx context.Context, a *EmployeeRecordAttribute) error
	DeleteRecordAttribute(ctx context.Context, recordID uuid.UUID, key string) error

	// Mappings
	GetMappingByID(ctx context.Context, id uuid.UUID) (*EmployeeProviderAttributeMapping, error)
	ListMappings(ctx context.Context, appID uuid.UUID, isDeterminator, allowUpstream *bool) ([]*EmployeeProviderAttributeMapping, error)
	InsertMapping(ctx context.Context, m *EmployeeProviderAttributeMapping) error
	DeleteMapping(ctx context.Context, id uuid.UUID) error

	// Matches
	GetMatchByRecord(ctx context.Context, recordID uuid.UUID) (*EmployeeRecordMatch, error)
	ListMatches(ctx context.Context) ([]*EmployeeRecordMatch, error)
	UpsertMatch(ctx context.Context, m *EmployeeRecordMatch) error
	DeleteMatch(ctx context.Context, recordID uuid.UUID) error

	// Cross-record peer traversal for upstream resolution: returns
	// records (other than `excludeRecordID`) that share at least one
	// attribute (key, value) with the given record, restricted to
	// mappings where allow_upstream = true for the record's
	// application.
	FindUpstreamPeers(ctx context.Context, recordID, excludeRecordID uuid.UUID) ([]uuid.UUID, error)

	// BulkUpsert reconciles a batch in one transaction.
	BulkUpsert(ctx context.Context, items []BulkItem, apps ApplicationResolver, idGen func() uuid.UUID) (int, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

func (r *BunRepository) GetRecordByID(ctx context.Context, id uuid.UUID) (*EmployeeRecord, error) {
	row := new(EmployeeRecord)
	err := r.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return row, nil
}

func (r *BunRepository) GetRecordByExternal(ctx context.Context, appID uuid.UUID, externalID string) (*EmployeeRecord, error) {
	row := new(EmployeeRecord)
	err := r.db.NewSelect().Model(row).
		Where("application_id = ?", appID).
		Where("external_id = ?", externalID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return row, nil
}

func (r *BunRepository) ListRecords(ctx context.Context) ([]*EmployeeRecord, error) {
	out := []*EmployeeRecord{}
	err := r.db.NewSelect().Model(&out).Order("external_id ASC").Scan(ctx)
	return out, err
}

func (r *BunRepository) InsertRecord(ctx context.Context, row *EmployeeRecord) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *BunRepository) ListRecordAttributes(ctx context.Context, recordID uuid.UUID) ([]*EmployeeRecordAttribute, error) {
	out := []*EmployeeRecordAttribute{}
	err := r.db.NewSelect().Model(&out).
		Where("employee_record_id = ?", recordID).
		Order("key ASC").Scan(ctx)
	return out, err
}

func (r *BunRepository) GetRecordAttribute(ctx context.Context, recordID uuid.UUID, key string) (*EmployeeRecordAttribute, error) {
	a := new(EmployeeRecordAttribute)
	err := r.db.NewSelect().Model(a).
		Where("employee_record_id = ?", recordID).
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

func (r *BunRepository) UpsertRecordAttribute(ctx context.Context, a *EmployeeRecordAttribute) error {
	_, err := r.db.NewInsert().
		Model(a).
		On("CONFLICT (employee_record_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

func (r *BunRepository) DeleteRecordAttribute(ctx context.Context, recordID uuid.UUID, key string) error {
	res, err := r.db.NewDelete().
		Model((*EmployeeRecordAttribute)(nil)).
		Where("employee_record_id = ?", recordID).
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

func (r *BunRepository) GetMappingByID(ctx context.Context, id uuid.UUID) (*EmployeeProviderAttributeMapping, error) {
	m := new(EmployeeProviderAttributeMapping)
	err := r.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMappingNotFound
		}
		return nil, err
	}
	return m, nil
}

func (r *BunRepository) ListMappings(ctx context.Context, appID uuid.UUID, isDeterminator, allowUpstream *bool) ([]*EmployeeProviderAttributeMapping, error) {
	out := []*EmployeeProviderAttributeMapping{}
	q := r.db.NewSelect().Model(&out).Where("application_id = ?", appID)
	if isDeterminator != nil {
		q = q.Where("is_determinator = ?", *isDeterminator)
	}
	if allowUpstream != nil {
		q = q.Where("allow_upstream = ?", *allowUpstream)
	}
	q = q.Order("employee_record_key ASC")
	err := q.Scan(ctx)
	return out, err
}

func (r *BunRepository) InsertMapping(ctx context.Context, m *EmployeeProviderAttributeMapping) error {
	_, err := r.db.NewInsert().Model(m).Exec(ctx)
	return err
}

func (r *BunRepository) DeleteMapping(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.NewDelete().Model((*EmployeeProviderAttributeMapping)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMappingNotFound
	}
	return nil
}

func (r *BunRepository) GetMatchByRecord(ctx context.Context, recordID uuid.UUID) (*EmployeeRecordMatch, error) {
	m := new(EmployeeRecordMatch)
	err := r.db.NewSelect().Model(m).Where("employee_record_id = ?", recordID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

func (r *BunRepository) ListMatches(ctx context.Context) ([]*EmployeeRecordMatch, error) {
	out := []*EmployeeRecordMatch{}
	err := r.db.NewSelect().Model(&out).Order("employee_record_id ASC").Scan(ctx)
	return out, err
}

func (r *BunRepository) UpsertMatch(ctx context.Context, m *EmployeeRecordMatch) error {
	_, err := r.db.NewInsert().
		Model(m).
		On("CONFLICT (employee_record_id) DO UPDATE").
		Set("person_id                = EXCLUDED.person_id").
		Set("employment_id            = EXCLUDED.employment_id").
		Set("matched_via_determinator = EXCLUDED.matched_via_determinator").
		Exec(ctx)
	return err
}

func (r *BunRepository) DeleteMatch(ctx context.Context, recordID uuid.UUID) error {
	_, err := r.db.NewDelete().
		Model((*EmployeeRecordMatch)(nil)).
		Where("employee_record_id = ?", recordID).
		Exec(ctx)
	return err
}

// FindUpstreamPeers returns peer record ids — records of the same
// application as `recordID` that share at least one (key, value) with
// it under an allow_upstream=true mapping, excluding `excludeRecordID`.
func (r *BunRepository) FindUpstreamPeers(ctx context.Context, recordID, excludeRecordID uuid.UUID) ([]uuid.UUID, error) {
	rows := []struct {
		PeerID uuid.UUID `bun:"peer_id"`
	}{}
	err := r.db.NewRaw(`
		WITH self_app AS (
			SELECT application_id FROM employee_records WHERE id = ?
		),
		upstream_keys AS (
			SELECT m.employee_record_key
			FROM employee_provider_attribute_mappings m
			JOIN self_app sa ON sa.application_id = m.application_id
			WHERE m.allow_upstream = TRUE
		),
		self_attrs AS (
			SELECT key, value
			FROM employee_record_attributes
			WHERE employee_record_id = ?
			  AND key IN (SELECT employee_record_key FROM upstream_keys)
		)
		SELECT DISTINCT era.employee_record_id AS peer_id
		FROM employee_record_attributes era
		JOIN self_attrs sa ON sa.key = era.key AND sa.value = era.value
		WHERE era.employee_record_id <> ?`,
		recordID, recordID, excludeRecordID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		out[i] = row.PeerID
	}
	return out, nil
}

func (r *BunRepository) BulkUpsert(ctx context.Context, items []BulkItem, apps ApplicationResolver, idGen func() uuid.UUID) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, it := range items {
		appID, ok, err := apps.ApplicationIDByCode(ctx, it.ApplicationCode)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, ErrApplicationNotFound
		}
		row := &EmployeeRecord{
			ID:            idGen(),
			ExternalID:    it.ExternalID,
			ApplicationID: appID,
			Description:   it.Description,
		}
		_, err = tx.NewInsert().Model(row).
			On("CONFLICT (application_id, external_id) DO UPDATE").
			Set("description = EXCLUDED.description").
			Returning("id").
			Exec(ctx)
		if err != nil {
			return 0, err
		}
		for k, v := range it.Attributes {
			a := &EmployeeRecordAttribute{
				ID:               idGen(),
				EmployeeRecordID: row.ID,
				Key:              k,
				Value:            v,
			}
			_, err := tx.NewInsert().Model(a).
				On("CONFLICT (employee_record_id, key) DO UPDATE").
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

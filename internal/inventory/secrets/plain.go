// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secrets

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Canonical plain-secret types. Mirrored by the CHECK constraint at the
// storage level.
//
//   - TypePassword:   an account password / shared secret.
//   - TypeConnString: a connection string (often embeds a password).
//   - TypeToken:      an issued bearer token (system or PAT — the
//     distinction is read from the linked principal's kind).
//   - TypeAPIKey:     a long-lived API key.
const (
	TypePassword   = "password"
	TypeConnString = "connstring"
	TypeToken      = "token"
	TypeAPIKey     = "api_key"
)

// SecretPlain is one piece of opaque secret material.
//
// Natural key: (Source, ExternalID). ExternalID is the provider's own
// identifier kept for traceability.
//
// Locus edge: TargetApplicationID + AccountID describe what the secret
// authenticates to and as; FoundInApplicationID + FoundInLocation
// describe where it was discovered. All are nullable.
//
// Fingerprint is a hash of the secret VALUE (never the value itself),
// used to correlate the same secret reused across locations or matched
// against a leak corpus.
//
// IssuedAt / ExpiresAt / RotatedAt / LastUsedAt are lifecycle facts; a
// NULL value means the source never evidenced that datum, so checks
// depending on it are not_evaluable (a Blind Spot).
type SecretPlain struct {
	bun.BaseModel `bun:"table:secret_plain,alias:sp"`

	ID                   uuid.UUID      `bun:"id,pk,type:uuid"               json:"id"`
	ExternalID           string         `bun:"external_id,notnull"           json:"external_id"`
	Source               string         `bun:"source,notnull"                json:"source"`
	Type                 string         `bun:"type,notnull"                  json:"type"`
	Label                string         `bun:"label,notnull"                 json:"label"`
	TargetApplicationID  *uuid.UUID     `bun:"target_application_id,type:uuid"   json:"target_application_id,omitempty"`
	AccountID            *uuid.UUID     `bun:"account_id,type:uuid"          json:"account_id,omitempty"`
	FoundInApplicationID *uuid.UUID     `bun:"found_in_application_id,type:uuid" json:"found_in_application_id,omitempty"`
	FoundInLocation      *string        `bun:"found_in_location"             json:"found_in_location,omitempty"`
	PrincipalID          *uuid.UUID     `bun:"principal_id,type:uuid"        json:"principal_id,omitempty"`
	Scopes               []string       `bun:"scopes,type:jsonb,notnull"     json:"scopes"`
	Fingerprint          *string        `bun:"fingerprint"                   json:"fingerprint,omitempty"`
	IsActive             bool           `bun:"is_active,notnull"             json:"is_active"`
	IsPrivileged         bool           `bun:"is_privileged,notnull"         json:"is_privileged"`
	IssuedAt             *time.Time     `bun:"issued_at"                     json:"issued_at,omitempty"`
	ExpiresAt            *time.Time     `bun:"expires_at"                    json:"expires_at,omitempty"`
	RotatedAt            *time.Time     `bun:"rotated_at"                    json:"rotated_at,omitempty"`
	LastUsedAt           *time.Time     `bun:"last_used_at"                  json:"last_used_at,omitempty"`
	Attrs                map[string]any `bun:"attrs,type:jsonb,notnull"  json:"attrs"`
	CreatedAt            time.Time      `bun:"created_at,notnull"            json:"created_at"`
	UpdatedAt            time.Time      `bun:"updated_at,notnull"            json:"updated_at"`
}

// PlainListFilter narrows what ListPlain returns. Zero value lists every
// row. Linked, when non-nil, narrows to secrets that do (true) or do not
// (false) resolve to a principal.
type PlainListFilter struct {
	TargetApplicationID  *uuid.UUID
	FoundInApplicationID *uuid.UUID
	AccountID            *uuid.UUID
	PrincipalID          *uuid.UUID
	Type                 string
	Privileged           *bool
	Linked               *bool
	Limit                int
	Offset               int
}

// PlainRepository is the persistence boundary for SecretPlain.
type PlainRepository interface {
	Upsert(ctx context.Context, tx bun.IDB, s *SecretPlain) error
	List(ctx context.Context, tx bun.IDB, f PlainListFilter) ([]*SecretPlain, int, error)
}

// PlainLookup resolves a single plain secret by id.
type PlainLookup interface {
	GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*SecretPlain, error)
}

// PlainBunRepository is the production Postgres-backed implementation of
// both PlainRepository and PlainLookup.
type PlainBunRepository struct{}

// NewPlainBunRepository constructs a PlainBunRepository.
func NewPlainBunRepository() *PlainBunRepository { return &PlainBunRepository{} }

// Upsert inserts a new SecretPlain or updates the existing one keyed by
// (source, external_id). updated_at is always refreshed.
func (r *PlainBunRepository) Upsert(ctx context.Context, tx bun.IDB, s *SecretPlain) error {
	if s.Scopes == nil {
		s.Scopes = []string{}
	}
	if s.Attrs == nil {
		s.Attrs = map[string]any{}
	}
	_, err := tx.NewInsert().
		Model(s).
		On("CONFLICT (source, external_id) DO UPDATE").
		Set("type                    = EXCLUDED.type").
		Set("label                   = EXCLUDED.label").
		Set("target_application_id   = EXCLUDED.target_application_id").
		Set("account_id              = EXCLUDED.account_id").
		Set("found_in_application_id = EXCLUDED.found_in_application_id").
		Set("found_in_location       = EXCLUDED.found_in_location").
		Set("principal_id            = EXCLUDED.principal_id").
		Set("scopes                  = EXCLUDED.scopes").
		Set("fingerprint             = EXCLUDED.fingerprint").
		Set("is_active               = EXCLUDED.is_active").
		Set("is_privileged           = EXCLUDED.is_privileged").
		Set("issued_at               = EXCLUDED.issued_at").
		Set("expires_at              = EXCLUDED.expires_at").
		Set("rotated_at              = EXCLUDED.rotated_at").
		Set("last_used_at            = EXCLUDED.last_used_at").
		Set("attrs                   = EXCLUDED.attrs").
		Set("updated_at              = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// List returns a paginated snapshot plus total row count honouring the
// filter. Default ordering is created_at ASC for a deterministic walk.
func (r *PlainBunRepository) List(ctx context.Context, tx bun.IDB, f PlainListFilter) ([]*SecretPlain, int, error) {
	out := []*SecretPlain{}
	q := tx.NewSelect().Model(&out)
	if f.TargetApplicationID != nil {
		q = q.Where("target_application_id = ?", *f.TargetApplicationID)
	}
	if f.FoundInApplicationID != nil {
		q = q.Where("found_in_application_id = ?", *f.FoundInApplicationID)
	}
	if f.AccountID != nil {
		q = q.Where("account_id = ?", *f.AccountID)
	}
	if f.PrincipalID != nil {
		q = q.Where("principal_id = ?", *f.PrincipalID)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.Privileged != nil {
		q = q.Where("is_privileged = ?", *f.Privileged)
	}
	if f.Linked != nil {
		if *f.Linked {
			q = q.Where("principal_id IS NOT NULL")
		} else {
			q = q.Where("principal_id IS NULL")
		}
	}
	q = q.Order("created_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	total, err := q.ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetByID returns the SecretPlain with the given primary key, or
// ErrNotFound.
func (r *PlainBunRepository) GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*SecretPlain, error) {
	s := new(SecretPlain)
	err := tx.NewSelect().Model(s).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

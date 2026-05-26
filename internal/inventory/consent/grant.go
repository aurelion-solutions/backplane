// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package consent

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Grant types. Mirrored by a storage CHECK.
//
//   - GrantDelegated:   on-behalf-of a signed-in user (delegated scopes).
//   - GrantApplication: app-only access (application permissions /
//     admin consent), acting as itself with no user in the loop.
const (
	GrantDelegated   = "delegated"
	GrantApplication = "application"
)

// ConsentGrant records that a subject granted a presented application a
// set of scopes.
//
// Natural key: (Source, ExternalID). ConsentedApplicationID is required —
// a grant with no application is meaningless. ConsentingPrincipalID is
// nullable: NULL means tenant-wide admin consent or an unresolved owner.
//
// Scopes are stored raw; whether any is high-risk is a policy verdict,
// not a stored fact. A NULL LastUsedAt makes a staleness check
// not_evaluable (a Blind Spot).
type ConsentGrant struct {
	bun.BaseModel `bun:"table:consent_grant,alias:cg"`

	ID                     uuid.UUID      `bun:"id,pk,type:uuid"                       json:"id"`
	Source                 string         `bun:"source,notnull"                        json:"source"`
	ExternalID             string         `bun:"external_id,notnull"                   json:"external_id"`
	ConsentedApplicationID uuid.UUID      `bun:"consented_application_id,type:uuid,notnull" json:"consented_application_id"`
	ConsentingPrincipalID  *uuid.UUID     `bun:"consenting_principal_id,type:uuid"     json:"consenting_principal_id,omitempty"`
	GrantType              string         `bun:"grant_type,notnull"                    json:"grant_type"`
	Scopes                 []string       `bun:"scopes,type:jsonb,notnull"             json:"scopes"`
	IsActive               bool           `bun:"is_active,notnull"                     json:"is_active"`
	GrantedAt              *time.Time     `bun:"granted_at"                            json:"granted_at,omitempty"`
	ExpiresAt              *time.Time     `bun:"expires_at"                            json:"expires_at,omitempty"`
	RevokedAt              *time.Time     `bun:"revoked_at"                            json:"revoked_at,omitempty"`
	LastUsedAt             *time.Time     `bun:"last_used_at"                          json:"last_used_at,omitempty"`
	Attrs                  map[string]any `bun:"attrs,type:jsonb,notnull"              json:"attrs"`
	CreatedAt              time.Time      `bun:"created_at,notnull"                    json:"created_at"`
	UpdatedAt              time.Time      `bun:"updated_at,notnull"                    json:"updated_at"`
}

// GrantListFilter narrows what ListGrants returns. Zero value lists every
// row. Owned, when non-nil, narrows to grants that do (true) or do not
// (false) name a consenting principal.
type GrantListFilter struct {
	ConsentedApplicationID *uuid.UUID
	ConsentingPrincipalID  *uuid.UUID
	GrantType              string
	Active                 *bool
	Owned                  *bool
	Limit                  int
	Offset                 int
}

// GrantRepository is the persistence boundary for ConsentGrant.
type GrantRepository interface {
	Upsert(ctx context.Context, tx bun.IDB, g *ConsentGrant) error
	List(ctx context.Context, tx bun.IDB, f GrantListFilter) ([]*ConsentGrant, int, error)
}

// GrantLookup resolves a single consent grant by id.
type GrantLookup interface {
	GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*ConsentGrant, error)
}

// GrantBunRepository is the production Postgres-backed implementation of
// both GrantRepository and GrantLookup.
type GrantBunRepository struct{}

// NewGrantBunRepository constructs a GrantBunRepository.
func NewGrantBunRepository() *GrantBunRepository { return &GrantBunRepository{} }

// Upsert inserts a new ConsentGrant or updates the existing one keyed by
// (source, external_id). updated_at is always refreshed.
func (r *GrantBunRepository) Upsert(ctx context.Context, tx bun.IDB, g *ConsentGrant) error {
	if g.Scopes == nil {
		g.Scopes = []string{}
	}
	if g.Attrs == nil {
		g.Attrs = map[string]any{}
	}
	_, err := tx.NewInsert().
		Model(g).
		On("CONFLICT (source, external_id) DO UPDATE").
		Set("consented_application_id = EXCLUDED.consented_application_id").
		Set("consenting_principal_id  = EXCLUDED.consenting_principal_id").
		Set("grant_type               = EXCLUDED.grant_type").
		Set("scopes                   = EXCLUDED.scopes").
		Set("is_active                = EXCLUDED.is_active").
		Set("granted_at               = EXCLUDED.granted_at").
		Set("expires_at               = EXCLUDED.expires_at").
		Set("revoked_at               = EXCLUDED.revoked_at").
		Set("last_used_at             = EXCLUDED.last_used_at").
		Set("attrs                    = EXCLUDED.attrs").
		Set("updated_at               = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// List returns a paginated snapshot plus total row count honouring the
// filter. Default ordering is created_at ASC for a deterministic walk.
func (r *GrantBunRepository) List(ctx context.Context, tx bun.IDB, f GrantListFilter) ([]*ConsentGrant, int, error) {
	out := []*ConsentGrant{}
	q := tx.NewSelect().Model(&out)
	if f.ConsentedApplicationID != nil {
		q = q.Where("consented_application_id = ?", *f.ConsentedApplicationID)
	}
	if f.ConsentingPrincipalID != nil {
		q = q.Where("consenting_principal_id = ?", *f.ConsentingPrincipalID)
	}
	if f.GrantType != "" {
		q = q.Where("grant_type = ?", f.GrantType)
	}
	if f.Active != nil {
		q = q.Where("is_active = ?", *f.Active)
	}
	if f.Owned != nil {
		if *f.Owned {
			q = q.Where("consenting_principal_id IS NOT NULL")
		} else {
			q = q.Where("consenting_principal_id IS NULL")
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

// GetByID returns the ConsentGrant with the given primary key, or
// ErrNotFound.
func (r *GrantBunRepository) GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*ConsentGrant, error) {
	g := new(ConsentGrant)
	err := tx.NewSelect().Model(g).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return g, nil
}

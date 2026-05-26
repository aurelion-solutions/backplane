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

// Resolution confidence — how sure the resolver is that the presented
// application maps to the linked principal. Mirrored by a storage CHECK.
//
//   - ConfidenceResolved:         confidently the linked principal.
//   - ConfidenceLikelySame:       strong but not certain match.
//   - ConfidenceAmbiguous:        multiple plausible matches.
//   - ConfidenceUnresolved:       no link established (the default).
//   - ConfidenceSpoofingSuspected: the self-asserted identity contradicts
//     the anchor (e.g. a name collision with a governed app under a
//     foreign client_id).
const (
	ConfidenceResolved          = "resolved"
	ConfidenceLikelySame        = "likely_same"
	ConfidenceAmbiguous         = "ambiguous"
	ConfidenceUnresolved        = "unresolved"
	ConfidenceSpoofingSuspected = "spoofing_suspected"
)

// Origin — first/third-party standing of the presented application,
// DERIVED from resolution, not asserted by the app. Mirrored by a CHECK.
const (
	OriginFirstParty = "first_party"
	OriginThirdParty = "third_party"
	OriginUnknown    = "unknown"
)

// ConsentedApplication is an application as it presented itself in a
// consent flow.
//
// Natural key: (Source, ClientID). ClientID is the IdP-issued anchor and
// the only field the app does not control. AppID is the registration id
// where the source distinguishes it from the client id.
//
// DisplayName / Publisher / HomeTenant / RedirectURIs are self-asserted
// claims. VerifiedPublisher is the one confirmed datum.
//
// ResolvedPrincipalID is set only when a resolver links the presented app
// to an already-governed identity; it never mints a new principal. Origin
// is derived from that resolution.
type ConsentedApplication struct {
	bun.BaseModel `bun:"table:consented_application,alias:ca"`

	ID                   uuid.UUID      `bun:"id,pk,type:uuid"                    json:"id"`
	Source               string         `bun:"source,notnull"                     json:"source"`
	ClientID             string         `bun:"client_id,notnull"                  json:"client_id"`
	AppID                *string        `bun:"app_id"                             json:"app_id,omitempty"`
	DisplayName          *string        `bun:"display_name"                       json:"display_name,omitempty"`
	Publisher            *string        `bun:"publisher"                          json:"publisher,omitempty"`
	VerifiedPublisher    bool           `bun:"verified_publisher,notnull"         json:"verified_publisher"`
	HomeTenant           *string        `bun:"home_tenant"                        json:"home_tenant,omitempty"`
	RedirectURIs         []string       `bun:"redirect_uris,type:jsonb,notnull"   json:"redirect_uris"`
	ResolvedPrincipalID  *uuid.UUID     `bun:"resolved_principal_id,type:uuid"    json:"resolved_principal_id,omitempty"`
	ResolutionConfidence string         `bun:"resolution_confidence,notnull"      json:"resolution_confidence"`
	Origin               string         `bun:"origin,notnull"                     json:"origin"`
	IsActive             bool           `bun:"is_active,notnull"                  json:"is_active"`
	Attrs                map[string]any `bun:"attrs,type:jsonb,notnull"           json:"attrs"`
	CreatedAt            time.Time      `bun:"created_at,notnull"                 json:"created_at"`
	UpdatedAt            time.Time      `bun:"updated_at,notnull"                 json:"updated_at"`
}

// AppListFilter narrows what ListApplications returns. Zero value lists
// every row. Resolved, when non-nil, narrows to apps that do (true) or do
// not (false) resolve to a principal.
type AppListFilter struct {
	ResolvedPrincipalID  *uuid.UUID
	Origin               string
	ResolutionConfidence string
	VerifiedPublisher    *bool
	Resolved             *bool
	Limit                int
	Offset               int
}

// AppRepository is the persistence boundary for ConsentedApplication.
type AppRepository interface {
	Upsert(ctx context.Context, tx bun.IDB, a *ConsentedApplication) error
	List(ctx context.Context, tx bun.IDB, f AppListFilter) ([]*ConsentedApplication, int, error)
}

// AppLookup resolves a single presented application by id.
type AppLookup interface {
	GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*ConsentedApplication, error)
}

// AppBunRepository is the production Postgres-backed implementation of
// both AppRepository and AppLookup.
type AppBunRepository struct{}

// NewAppBunRepository constructs an AppBunRepository.
func NewAppBunRepository() *AppBunRepository { return &AppBunRepository{} }

// Upsert inserts a new ConsentedApplication or updates the existing one
// keyed by (source, client_id). updated_at is always refreshed.
func (r *AppBunRepository) Upsert(ctx context.Context, tx bun.IDB, a *ConsentedApplication) error {
	if a.RedirectURIs == nil {
		a.RedirectURIs = []string{}
	}
	if a.Attrs == nil {
		a.Attrs = map[string]any{}
	}
	if a.ResolutionConfidence == "" {
		a.ResolutionConfidence = ConfidenceUnresolved
	}
	if a.Origin == "" {
		a.Origin = OriginUnknown
	}
	_, err := tx.NewInsert().
		Model(a).
		On("CONFLICT (source, client_id) DO UPDATE").
		Set("app_id                = EXCLUDED.app_id").
		Set("display_name          = EXCLUDED.display_name").
		Set("publisher             = EXCLUDED.publisher").
		Set("verified_publisher    = EXCLUDED.verified_publisher").
		Set("home_tenant           = EXCLUDED.home_tenant").
		Set("redirect_uris         = EXCLUDED.redirect_uris").
		Set("resolved_principal_id = EXCLUDED.resolved_principal_id").
		Set("resolution_confidence = EXCLUDED.resolution_confidence").
		Set("origin                = EXCLUDED.origin").
		Set("is_active             = EXCLUDED.is_active").
		Set("attrs                 = EXCLUDED.attrs").
		Set("updated_at            = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// List returns a paginated snapshot plus total row count honouring the
// filter. Default ordering is created_at ASC for a deterministic walk.
func (r *AppBunRepository) List(ctx context.Context, tx bun.IDB, f AppListFilter) ([]*ConsentedApplication, int, error) {
	out := []*ConsentedApplication{}
	q := tx.NewSelect().Model(&out)
	if f.ResolvedPrincipalID != nil {
		q = q.Where("resolved_principal_id = ?", *f.ResolvedPrincipalID)
	}
	if f.Origin != "" {
		q = q.Where("origin = ?", f.Origin)
	}
	if f.ResolutionConfidence != "" {
		q = q.Where("resolution_confidence = ?", f.ResolutionConfidence)
	}
	if f.VerifiedPublisher != nil {
		q = q.Where("verified_publisher = ?", *f.VerifiedPublisher)
	}
	if f.Resolved != nil {
		if *f.Resolved {
			q = q.Where("resolved_principal_id IS NOT NULL")
		} else {
			q = q.Where("resolved_principal_id IS NULL")
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

// GetByID returns the ConsentedApplication with the given primary key, or
// ErrNotFound.
func (r *AppBunRepository) GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*ConsentedApplication, error) {
	a := new(ConsentedApplication)
	err := tx.NewSelect().Model(a).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

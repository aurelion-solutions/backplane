// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_profile

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the read boundary for the access-profile projection.
// Every method is a plain SELECT — no writes, no transactions.
type Repository interface {
	Person(ctx context.Context, id uuid.UUID) (*personRow, error)
	Employments(ctx context.Context, personID uuid.UUID) ([]employmentRow, error)
	EmploymentPrincipals(ctx context.Context, employmentIDs []uuid.UUID) ([]principalRow, error)
	Accounts(ctx context.Context, principalIDs []uuid.UUID) ([]accountRow, error)
	Grants(ctx context.Context, accountIDs []uuid.UUID) ([]grantRow, error)
	Initiatives(ctx context.Context, principalIDs []uuid.UUID) ([]initiativeRow, error)
}

// Flat row shapes scanned out of bun. They stay unexported — the
// service folds them into the public *View documents in schemas.go.

type personRow struct {
	ID         uuid.UUID `bun:"id"`
	ExternalID string    `bun:"external_id"`
	FullName   string    `bun:"full_name"`
}

type employmentRow struct {
	ID        uuid.UUID  `bun:"id"`
	Code      string     `bun:"code"`
	StartDate time.Time  `bun:"start_date"`
	EndDate   *time.Time `bun:"end_date"`
}

type principalRow struct {
	ID           uuid.UUID `bun:"id"`
	EmploymentID uuid.UUID `bun:"principal_employment_id"`
}

type accountRow struct {
	ID              uuid.UUID `bun:"id"`
	PrincipalID     uuid.UUID `bun:"principal_id"`
	ApplicationID   uuid.UUID `bun:"application_id"`
	ApplicationName string    `bun:"application_name"`
	ApplicationCode string    `bun:"application_code"`
	Username        string    `bun:"username"`
	DisplayName     *string   `bun:"display_name"`
	IsActive        bool      `bun:"is_active"`
	IsPrivileged    bool      `bun:"is_privileged"`
	MFAEnabled      bool      `bun:"mfa_enabled"`
	EffectiveState  string    `bun:"effective_state"`
}

type grantRow struct {
	AccountID      uuid.UUID `bun:"account_id"`
	CapabilitySlug string    `bun:"capability_slug"`
	CapabilityName string    `bun:"capability_name"`
	ScopeKeyCode   string    `bun:"scope_key_code"`
	ScopeValue     *string   `bun:"scope_value"`
}

type initiativeRow struct {
	ID              uuid.UUID  `bun:"id"`
	PrincipalID     uuid.UUID  `bun:"principal_id"`
	ApplicationID   uuid.UUID  `bun:"application_id"`
	ApplicationName string     `bun:"application_name"`
	ApplicationCode string     `bun:"application_code"`
	CapabilityName  *string    `bun:"capability_name"`
	Kind            string     `bun:"kind"`
	Actor           string     `bun:"actor"`
	ValidFrom       time.Time  `bun:"valid_from"`
	ValidUntil      *time.Time `bun:"valid_until"`
}

// BunRepository is the Postgres-backed implementation.
type BunRepository struct{ db *bun.DB }

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository { return &BunRepository{db: db} }

// Person resolves the person root, or ErrPersonNotFound.
func (r *BunRepository) Person(ctx context.Context, id uuid.UUID) (*personRow, error) {
	row := new(personRow)
	err := r.db.NewSelect().
		Table("persons").
		Column("id", "external_id", "full_name").
		Where("id = ?", id).
		Scan(ctx, row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPersonNotFound
		}
		return nil, err
	}
	return row, nil
}

// Employments lists the person's working periods, oldest first.
func (r *BunRepository) Employments(ctx context.Context, personID uuid.UUID) ([]employmentRow, error) {
	var rows []employmentRow
	err := r.db.NewSelect().
		Table("employments").
		Column("id", "code", "start_date", "end_date").
		Where("person_id = ?", personID).
		Order("start_date ASC").
		Scan(ctx, &rows)
	return rows, err
}

// EmploymentPrincipals returns the employment-kind principals bound to
// the given employments.
func (r *BunRepository) EmploymentPrincipals(ctx context.Context, employmentIDs []uuid.UUID) ([]principalRow, error) {
	if len(employmentIDs) == 0 {
		return nil, nil
	}
	var rows []principalRow
	err := r.db.NewSelect().
		Table("principals").
		Column("id", "principal_employment_id").
		Where("kind = ?", "employment").
		Where("principal_employment_id IN (?)", bun.In(employmentIDs)).
		Scan(ctx, &rows)
	return rows, err
}

// Accounts returns the accounts assigned to the given principals,
// joined to their application for human-facing labels.
func (r *BunRepository) Accounts(ctx context.Context, principalIDs []uuid.UUID) ([]accountRow, error) {
	if len(principalIDs) == 0 {
		return nil, nil
	}
	var rows []accountRow
	err := r.db.NewSelect().
		TableExpr("accounts AS acc").
		ColumnExpr("acc.id AS id").
		ColumnExpr("acc.principal_id AS principal_id").
		ColumnExpr("acc.application_id AS application_id").
		ColumnExpr("app.name AS application_name").
		ColumnExpr("app.code AS application_code").
		ColumnExpr("acc.username AS username").
		ColumnExpr("acc.display_name AS display_name").
		ColumnExpr("acc.is_active AS is_active").
		ColumnExpr("acc.is_privileged AS is_privileged").
		ColumnExpr("acc.mfa_enabled AS mfa_enabled").
		ColumnExpr("acc.effective_state AS effective_state").
		Join("JOIN applications AS app ON app.id = acc.application_id").
		Where("acc.principal_id IN (?)", bun.In(principalIDs)).
		Order("app.name ASC", "acc.username ASC").
		Scan(ctx, &rows)
	return rows, err
}

// Grants returns the live (non-tombstoned) capability grants on the
// given accounts, joined to the catalog for labels.
func (r *BunRepository) Grants(ctx context.Context, accountIDs []uuid.UUID) ([]grantRow, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	var rows []grantRow
	err := r.db.NewSelect().
		TableExpr("capability_grants AS cg").
		ColumnExpr("cg.account_id AS account_id").
		ColumnExpr("cap.slug AS capability_slug").
		ColumnExpr("cap.name AS capability_name").
		ColumnExpr("csk.code AS scope_key_code").
		ColumnExpr("cg.scope_value AS scope_value").
		Join("JOIN capabilities AS cap ON cap.id = cg.capability_id").
		Join("JOIN capability_scope_keys AS csk ON csk.id = cg.scope_key_id").
		Where("cg.account_id IN (?)", bun.In(accountIDs)).
		Where("cg.tombstoned_at IS NULL").
		Order("cap.name ASC").
		Scan(ctx, &rows)
	return rows, err
}

// Initiatives returns the live (non-tombstoned) justifications for the
// given principals, joined to their application and (optionally) the
// capability they target.
func (r *BunRepository) Initiatives(ctx context.Context, principalIDs []uuid.UUID) ([]initiativeRow, error) {
	if len(principalIDs) == 0 {
		return nil, nil
	}
	var rows []initiativeRow
	err := r.db.NewSelect().
		TableExpr("initiatives AS init").
		ColumnExpr("init.id AS id").
		ColumnExpr("init.principal_id AS principal_id").
		ColumnExpr("init.application_id AS application_id").
		ColumnExpr("app.name AS application_name").
		ColumnExpr("app.code AS application_code").
		ColumnExpr("cap.name AS capability_name").
		ColumnExpr("init.kind AS kind").
		ColumnExpr("init.actor AS actor").
		ColumnExpr("init.valid_from AS valid_from").
		ColumnExpr("init.valid_until AS valid_until").
		Join("JOIN applications AS app ON app.id = init.application_id").
		Join("LEFT JOIN capabilities AS cap ON cap.id = init.capability_id").
		Where("init.principal_id IN (?)", bun.In(principalIDs)).
		Where("init.tombstoned_at IS NULL").
		Order("init.valid_until ASC NULLS LAST").
		Scan(ctx, &rows)
	return rows, err
}

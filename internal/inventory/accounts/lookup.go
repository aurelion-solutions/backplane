// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package accounts

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ErrNotFound is returned by GetByApplicationAndUsername when no
// matching account exists.
var ErrNotFound = errors.New("accounts: not found")

// Lookup is the read-side persistence boundary the access projector
// needs: resolve an account by its (application, username) natural
// key. Separate from Repository to keep the write-side narrow and
// make stubbing in tests trivial.
type Lookup interface {
	GetByApplicationAndUsername(ctx context.Context, tx bun.IDB, applicationID uuid.UUID, username string) (*Account, error)
}

// LookupBunRepository is the Postgres-backed Lookup implementation.
// Reads ignore tx-state by going through the supplied bun.IDB so the
// action's runner transaction sees its own pending writes too.
type LookupBunRepository struct{}

// NewLookupBunRepository constructs a LookupBunRepository.
func NewLookupBunRepository() *LookupBunRepository {
	return &LookupBunRepository{}
}

// GetByApplicationAndUsername returns the matching Account, or
// ErrNotFound.
func (l *LookupBunRepository) GetByApplicationAndUsername(ctx context.Context, tx bun.IDB, applicationID uuid.UUID, username string) (*Account, error) {
	a := new(Account)
	err := tx.NewSelect().
		Model(a).
		Where("application_id = ?", applicationID).
		Where("username = ?", username).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// Slice 2a evidence-presence signals, same pattern as
			// mfa_evidence_at. NULL = evidence absent ⇒ the dependent
			// data-quality check is not_evaluable (a Blind Spot).
			_, err := db.ExecContext(ctx, `
				ALTER TABLE accounts
					ADD COLUMN owner_evidence_at     TIMESTAMPTZ NULL,
					ADD COLUMN last_used_evidence_at TIMESTAMPTZ NULL;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE accounts
					DROP COLUMN IF EXISTS owner_evidence_at,
					DROP COLUMN IF EXISTS last_used_evidence_at;
			`)
			return err
		},
	)
}

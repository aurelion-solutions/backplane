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
			// Evidence-presence for MFA. NULL = no MFA evidence ⇒
			// MFA-dependent checks are not_evaluable. mfa_enabled (the
			// value) is untouched; this column records whether a source
			// actually evidenced it.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE accounts
					ADD COLUMN mfa_evidence_at TIMESTAMPTZ NULL;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE accounts
					DROP COLUMN IF EXISTS mfa_evidence_at;
			`)
			return err
		},
	)
}

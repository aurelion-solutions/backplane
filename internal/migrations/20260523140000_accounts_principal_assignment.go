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
			// Account → Principal assignment edge. An account is a
			// provider mailbox; principal_id records which governed
			// identity holds it. NULL = unassigned (an orphan account,
			// itself a posture signal). Mirrors workloads.owner_employment_id.
			// ON DELETE SET NULL: deleting a principal must not erase the
			// observed account, only its assignment.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE accounts
					ADD COLUMN IF NOT EXISTS principal_id uuid
					REFERENCES principals(id) ON DELETE SET NULL;
				CREATE INDEX IF NOT EXISTS idx_accounts_principal_id
					ON accounts (principal_id) WHERE principal_id IS NOT NULL;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP INDEX IF EXISTS idx_accounts_principal_id;
				ALTER TABLE accounts DROP COLUMN IF EXISTS principal_id;
			`)
			return err
		},
	)
}

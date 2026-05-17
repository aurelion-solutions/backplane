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
			_, err := db.ExecContext(ctx, `
				ALTER TABLE accounts
					ADD COLUMN desired_state    TEXT NOT NULL DEFAULT 'pending',
					ADD COLUMN validated_state  TEXT NOT NULL DEFAULT 'pending',
					ADD COLUMN effective_state  TEXT NOT NULL DEFAULT 'pending';

				UPDATE accounts SET effective_state = CASE
					WHEN is_active THEN 'active'
					ELSE 'blocked'
				END;

				ALTER TABLE accounts ADD CONSTRAINT chk_accounts_desired_state
					CHECK (desired_state IN ('not_exist', 'pending', 'blocked', 'invited', 'active'));
				ALTER TABLE accounts ADD CONSTRAINT chk_accounts_validated_state
					CHECK (validated_state IN ('not_exist', 'pending', 'blocked', 'invited', 'active'));
				ALTER TABLE accounts ADD CONSTRAINT chk_accounts_effective_state
					CHECK (effective_state IN ('not_exist', 'pending', 'blocked', 'invited', 'active'));

				CREATE INDEX ix_accounts_effective_state ON accounts(effective_state);
				CREATE INDEX ix_accounts_desired_state   ON accounts(desired_state);
				CREATE INDEX ix_accounts_validated_state ON accounts(validated_state);

				-- access_apply scans for rows where the validated column
				-- diverges from the effective one; the composite index
				-- supports that predicate within an application.
				CREATE INDEX ix_accounts_state_divergence
					ON accounts(application_id, validated_state, effective_state);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP INDEX IF EXISTS ix_accounts_state_divergence;
				DROP INDEX IF EXISTS ix_accounts_validated_state;
				DROP INDEX IF EXISTS ix_accounts_desired_state;
				DROP INDEX IF EXISTS ix_accounts_effective_state;

				ALTER TABLE accounts DROP CONSTRAINT IF EXISTS chk_accounts_effective_state;
				ALTER TABLE accounts DROP CONSTRAINT IF EXISTS chk_accounts_validated_state;
				ALTER TABLE accounts DROP CONSTRAINT IF EXISTS chk_accounts_desired_state;

				ALTER TABLE accounts
					DROP COLUMN IF EXISTS effective_state,
					DROP COLUMN IF EXISTS validated_state,
					DROP COLUMN IF EXISTS desired_state;
			`)
			return err
		},
	)
}

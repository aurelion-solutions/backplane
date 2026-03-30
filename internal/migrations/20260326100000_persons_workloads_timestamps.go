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
				ALTER TABLE persons
					ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
				CREATE INDEX ix_persons_updated_at ON persons(updated_at DESC);

				ALTER TABLE workloads
					ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
				CREATE INDEX ix_workloads_updated_at ON workloads(updated_at DESC);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP INDEX IF EXISTS ix_persons_updated_at;
				ALTER TABLE persons
					DROP COLUMN IF EXISTS created_at,
					DROP COLUMN IF EXISTS updated_at;

				DROP INDEX IF EXISTS ix_workloads_updated_at;
				ALTER TABLE workloads
					DROP COLUMN IF EXISTS created_at,
					DROP COLUMN IF EXISTS updated_at;
			`)
			return err
		},
	)
}

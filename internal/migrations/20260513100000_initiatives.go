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
				CREATE TABLE initiatives (
					id              UUID PRIMARY KEY,
					principal_id    UUID NOT NULL REFERENCES principals(id) ON DELETE RESTRICT,
					application_id  UUID NOT NULL REFERENCES applications(id) ON DELETE RESTRICT,
					capability_id   UUID          REFERENCES capabilities(id) ON DELETE RESTRICT,
					kind            TEXT NOT NULL,
					justification   JSONB NOT NULL DEFAULT '{}',
					actor           TEXT NOT NULL,
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

					-- Validity window. Typical case: both NULL on insert,
					-- valid_from gets NOW() via column default. Future-
					-- dated case ("c понедельника на две недели"):
					-- caller sets both explicitly.
					valid_from      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					valid_until     TIMESTAMPTZ,

					-- Hard withdrawal. tombstoned_at takes precedence
					-- over any future-dated validity window.
					tombstoned_at   TIMESTAMPTZ
				);

				-- Multiple active initiatives per target are allowed:
				-- access ⇐ any single active justification. No partial
				-- unique index on the active set.

				CREATE INDEX ix_initiatives_principal   ON initiatives (principal_id, tombstoned_at);
				CREATE INDEX ix_initiatives_application ON initiatives (application_id, tombstoned_at);
				CREATE INDEX ix_initiatives_active
					ON initiatives (tombstoned_at, valid_until)
					WHERE tombstoned_at IS NULL;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS initiatives;
			`)
			return err
		},
	)
}

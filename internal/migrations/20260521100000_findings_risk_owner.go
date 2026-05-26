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
			// Slice 3 triage denormalisation. Findings gain the
			// attributes the assessment UI filters and groups by —
			// application / source / cartridge / owner — plus a
			// factor-decomposed priority score, all stamped at assess
			// time so a finding is self-describing without joins.
			//
			// owner on applications is inventory data (who owns this
			// system), not UI-managed config; a finding inherits its
			// account's application owner for routing.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE findings
					ADD COLUMN application_id   UUID    NULL,
					ADD COLUMN source           TEXT    NULL,
					ADD COLUMN cartridge_ref    TEXT    NULL,
					ADD COLUMN owner_ref        TEXT    NULL,
					ADD COLUMN priority_score   INTEGER NOT NULL DEFAULT 0,
					ADD COLUMN priority_factors JSONB   NOT NULL DEFAULT '[]'::jsonb;

				ALTER TABLE applications
					ADD COLUMN owner TEXT NULL;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE findings
					DROP COLUMN IF EXISTS application_id,
					DROP COLUMN IF EXISTS source,
					DROP COLUMN IF EXISTS cartridge_ref,
					DROP COLUMN IF EXISTS owner_ref,
					DROP COLUMN IF EXISTS priority_score,
					DROP COLUMN IF EXISTS priority_factors;

				ALTER TABLE applications
					DROP COLUMN IF EXISTS owner;
			`)
			return err
		},
	)
}

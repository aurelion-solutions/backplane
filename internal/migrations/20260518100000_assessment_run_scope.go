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
			// Slice 1 temporal anchors. as_of is the point in time the
			// run reflects; period_{start,end} stay NULL until the
			// Slice 5 / evidence-chain M2 period query uses them. Laid
			// down now so the anchor exists and is never retrofitted.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE policy_assessment_runs
					ADD COLUMN as_of        TIMESTAMPTZ NULL,
					ADD COLUMN period_start TIMESTAMPTZ NULL,
					ADD COLUMN period_end   TIMESTAMPTZ NULL,
					ADD COLUMN outcomes_by_kind JSONB NOT NULL DEFAULT '{}';
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE policy_assessment_runs
					DROP COLUMN IF EXISTS as_of,
					DROP COLUMN IF EXISTS period_start,
					DROP COLUMN IF EXISTS period_end,
					DROP COLUMN IF EXISTS outcomes_by_kind;
			`)
			return err
		},
	)
}

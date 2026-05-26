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
			// last_seen_run_id is the most recent run that re-confirmed a
			// finding. Backfill it to assessment_run_id (first detection)
			// so every existing row is consistent, then make it NOT NULL.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE findings ADD COLUMN last_seen_run_id UUID;
				UPDATE findings SET last_seen_run_id = assessment_run_id WHERE last_seen_run_id IS NULL;
				ALTER TABLE findings ALTER COLUMN last_seen_run_id SET NOT NULL;
				CREATE INDEX ix_findings_last_seen_run_id ON findings(last_seen_run_id);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP INDEX IF EXISTS ix_findings_last_seen_run_id;
				ALTER TABLE findings DROP COLUMN IF EXISTS last_seen_run_id;
			`)
			return err
		},
	)
}

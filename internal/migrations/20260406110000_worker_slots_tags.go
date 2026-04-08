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
				-- Readonly tags carried by every slot of a worker process.
				-- Sourced from AURELION_WORKER_TAGS at worker startup
				-- (CSV string → array). Shared across all slots of one
				-- process by construction. Purely informational for now —
				-- the Studio overview panel renders them as chips next to
				-- the PID header.
				ALTER TABLE worker_slots
					ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE worker_slots DROP COLUMN tags;
			`)
			return err
		},
	)
}

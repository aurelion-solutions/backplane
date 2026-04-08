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
				-- ----------------------------------------------------------
				-- worker_slots
				--
				-- Registry of runner slots that are currently alive. One row
				-- per (hostname, pid, slot_index). The runner UPSERTs on
				-- startup, ticks last_heartbeat_at on a fixed cadence, and
				-- DELETEs on graceful shutdown. The /workers endpoint reads
				-- this table directly (no JOIN through pipeline_runs) so
				-- idle slots are visible — derived-from-runs view never
				-- shows idle workers, which was the original gap.
				-- ----------------------------------------------------------

				CREATE TABLE worker_slots (
					worker_id          VARCHAR(255)  PRIMARY KEY,
					hostname           VARCHAR(255)  NOT NULL,
					pid                INTEGER       NOT NULL,
					slot_index         INTEGER       NOT NULL,
					started_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
					last_heartbeat_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
				);

				CREATE INDEX ix_worker_slots_heartbeat ON worker_slots(last_heartbeat_at);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS worker_slots;`)
			return err
		},
	)
}

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
				CREATE TABLE inventory_discover_runs (
					id                     UUID         PRIMARY KEY,
					connector_instance_id  VARCHAR(255) NOT NULL,
					operation              VARCHAR(128) NOT NULL,
					dataset_type           VARCHAR(128) NOT NULL,
					correlation_id         VARCHAR(64)  NOT NULL,
					status                 VARCHAR(32)  NOT NULL,
					error                  TEXT,
					received_count         INTEGER      NOT NULL DEFAULT 0,
					written_count          INTEGER      NOT NULL DEFAULT 0,
					started_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					completed_at           TIMESTAMPTZ,
					CONSTRAINT ck_inventory_discover_runs_status CHECK (
						status IN ('dispatched','running','completed','failed','timed_out')
					)
				);
				CREATE INDEX ix_inventory_discover_runs_connector   ON inventory_discover_runs(connector_instance_id);
				CREATE INDEX ix_inventory_discover_runs_correlation ON inventory_discover_runs(correlation_id);
				CREATE INDEX ix_inventory_discover_runs_started     ON inventory_discover_runs(started_at DESC);
				CREATE INDEX ix_inventory_discover_runs_status      ON inventory_discover_runs(status);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS inventory_discover_runs;
			`)
			return err
		},
	)
}

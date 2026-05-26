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
				CREATE TABLE workload_lineage_snapshots (
					id          UUID        PRIMARY KEY,
					workload_id UUID        NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
					resolved_at TIMESTAMPTZ NOT NULL,
					terminus    VARCHAR(32) NOT NULL,
					chain       JSONB       NOT NULL,
					chain_hash  VARCHAR(64) NOT NULL,
					created_at  TIMESTAMPTZ NOT NULL,
					CONSTRAINT uq_wls_workload_chainhash UNIQUE (workload_id, chain_hash)
				);
				CREATE INDEX ix_wls_workload_id ON workload_lineage_snapshots(workload_id);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS workload_lineage_snapshots;`)
			return err
		},
	)
}

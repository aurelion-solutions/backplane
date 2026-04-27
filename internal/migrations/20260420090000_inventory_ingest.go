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
				CREATE TABLE inventory_ingest_batches (
					id              UUID         PRIMARY KEY,
					source          VARCHAR(64)  NOT NULL,
					dataset_type    VARCHAR(128) NOT NULL,
					correlation_id  VARCHAR(64)  NOT NULL,
					received_count  INTEGER      NOT NULL,
					written_count   INTEGER      NOT NULL,
					skipped_count   INTEGER      NOT NULL,
					new_count       INTEGER      NOT NULL,
					changed_count   INTEGER      NOT NULL,
					lake_ref        TEXT,
					completed_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
				);
				CREATE INDEX ix_inventory_ingest_batches_source        ON inventory_ingest_batches(source);
				CREATE INDEX ix_inventory_ingest_batches_dataset_type  ON inventory_ingest_batches(dataset_type);
				CREATE INDEX ix_inventory_ingest_batches_correlation   ON inventory_ingest_batches(correlation_id);
				CREATE INDEX ix_inventory_ingest_batches_completed_at  ON inventory_ingest_batches(completed_at DESC);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS inventory_ingest_batches;
			`)
			return err
		},
	)
}

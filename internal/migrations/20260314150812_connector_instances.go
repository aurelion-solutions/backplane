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
				CREATE TABLE connector_instances (
					id            UUID         PRIMARY KEY,
					instance_id   VARCHAR(255) NOT NULL,
					tags          JSONB        NOT NULL DEFAULT '[]'::jsonb,
					descriptor    JSONB,
					last_seen_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_connector_instances_instance_id UNIQUE (instance_id)
				);
				CREATE INDEX ix_connector_instances_instance_id ON connector_instances(instance_id);
				CREATE INDEX ix_connector_instances_last_seen_at ON connector_instances(last_seen_at);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS connector_instances;`)
			return err
		},
	)
}

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
				CREATE TABLE org_units (
					id           UUID         PRIMARY KEY,
					external_id  VARCHAR(255) NOT NULL,
					name         VARCHAR(255) NOT NULL,
					parent_id    UUID         REFERENCES org_units(id) ON DELETE SET NULL,
					description  TEXT,
					is_internal  BOOLEAN      NOT NULL DEFAULT FALSE,
					created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_org_units_external_id UNIQUE (external_id)
				);
				CREATE INDEX ix_org_units_external_id ON org_units(external_id);
				CREATE INDEX ix_org_units_parent_id    ON org_units(parent_id);
				CREATE INDEX ix_org_units_is_internal  ON org_units(is_internal);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS org_units;`)
			return err
		},
	)
}

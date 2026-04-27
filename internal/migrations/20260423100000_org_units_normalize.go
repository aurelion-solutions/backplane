// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

// Extends org_units with the columns required by the orgunit ingest
// contract: human-readable display_name and is_active soft-delete
// flag. Other contract fields (label, company, manager, meta) are
// deferred — they need their own EAV/JSONB layer.
func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE org_units
					ADD COLUMN display_name VARCHAR(255) NOT NULL DEFAULT '',
					ADD COLUMN is_active    BOOLEAN      NOT NULL DEFAULT TRUE;
				CREATE INDEX ix_org_units_is_active ON org_units(is_active);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP INDEX IF EXISTS ix_org_units_is_active;
				ALTER TABLE org_units
					DROP COLUMN IF EXISTS is_active,
					DROP COLUMN IF EXISTS display_name;
			`)
			return err
		},
	)
}

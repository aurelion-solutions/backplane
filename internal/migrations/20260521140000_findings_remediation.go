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
			// Suggested action + remediation guidance, denormalised from
			// the cartridge finding metadata at assess time so the
			// assessment UI shows a cartridge-sourced next step.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE findings
					ADD COLUMN recommended_action TEXT NULL,
					ADD COLUMN remediation        TEXT NULL;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE findings
					DROP COLUMN IF EXISTS recommended_action,
					DROP COLUMN IF EXISTS remediation;
			`)
			return err
		},
	)
}

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
				-- Tags are coarse facets used by the runtime to pre-filter
				-- applicable policies before dispatching to mechanism
				-- handlers. Free-form strings ("authn", "transport:saml",
				-- "geo:eu", "scan", "framework:sox"); matching is subset
				-- containment ("every tag in policy.tags must appear in
				-- request.facets").
				ALTER TABLE policies
					ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';

				CREATE INDEX ix_policies_tags
					ON policies USING GIN (tags)
					WHERE is_active = TRUE;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP INDEX IF EXISTS ix_policies_tags;
				ALTER TABLE policies DROP COLUMN IF EXISTS tags;
			`)
			return err
		},
	)
}

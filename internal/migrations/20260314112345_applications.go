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
				CREATE TABLE applications (
					id                       UUID         PRIMARY KEY,
					name                     VARCHAR(255) NOT NULL,
					code                     VARCHAR(64)  NOT NULL,
					config                   JSONB        NOT NULL DEFAULT '{}'::jsonb,
					required_connector_tags  JSONB        NOT NULL DEFAULT '[]'::jsonb,
					is_active                BOOLEAN      NOT NULL DEFAULT TRUE,
					created_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_applications_name UNIQUE (name),
					CONSTRAINT uq_applications_code UNIQUE (code)
				);
				CREATE INDEX ix_applications_name ON applications(name);
				CREATE INDEX ix_applications_code ON applications(code);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS applications;`)
			return err
		},
	)
}

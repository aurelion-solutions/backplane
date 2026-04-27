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
				CREATE TABLE accounts (
					id              UUID         PRIMARY KEY,
					application_id  UUID         NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
					username        VARCHAR(255) NOT NULL,
					external_id     VARCHAR(255) NOT NULL,
					source          VARCHAR(64)  NOT NULL,
					display_name    VARCHAR(255),
					email           VARCHAR(255),
					is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
					is_privileged   BOOLEAN      NOT NULL DEFAULT FALSE,
					mfa_enabled     BOOLEAN      NOT NULL DEFAULT FALSE,
					status          VARCHAR(64),
					attrs           JSONB        NOT NULL DEFAULT '{}',
					created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
				);
				CREATE UNIQUE INDEX uq_accounts_application_username ON accounts(application_id, username);
				CREATE INDEX ix_accounts_application_id ON accounts(application_id);
				CREATE INDEX ix_accounts_external_id    ON accounts(external_id);
				CREATE INDEX ix_accounts_source         ON accounts(source);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS accounts;
			`)
			return err
		},
	)
}

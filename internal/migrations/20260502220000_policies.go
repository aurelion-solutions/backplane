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
				-- PG mirror of cartridge-defined policy rules. The cartridge
				-- filesystem is the source of truth; this table is a
				-- projection rebuilt by the sync loop so the rest of the
				-- system can query / reference rules by stable id without
				-- walking the cartridge tree.
				--
				-- Rego bodies are NOT mirrored — only metadata. Forensic
				-- re-evaluation of removed rule bodies will live in a
				-- separate policy_versions table when needed.
				CREATE TABLE policies (
					id             UUID         PRIMARY KEY,
					cartridge_ref  VARCHAR(128) NOT NULL,
					rule_id        VARCHAR(255) NOT NULL,
					name           VARCHAR(255) NOT NULL,
					description    TEXT,
					mechanism      VARCHAR(64)  NOT NULL,
					severity       VARCHAR(32),
					owner_team     VARCHAR(128),
					version        INTEGER      NOT NULL DEFAULT 1,
					is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
					removed_at     TIMESTAMPTZ,
					meta           JSONB        NOT NULL DEFAULT '{}'::jsonb,
					created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_policies_natural UNIQUE (cartridge_ref, rule_id)
				);
				CREATE INDEX ix_policies_mechanism ON policies(mechanism)
					WHERE is_active = TRUE;
				CREATE INDEX ix_policies_active    ON policies(is_active)
					WHERE is_active = TRUE;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS policies;
			`)
			return err
		},
	)
}

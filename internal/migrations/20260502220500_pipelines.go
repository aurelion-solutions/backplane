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
				-- PG mirror of cartridge-defined pipeline definitions.
				-- Mirrors policies in shape: the cartridge filesystem is
				-- source of truth, this table is a projection rebuilt by
				-- the sync loop.
				--
				-- The YAML body is NOT mirrored — only metadata + a
				-- content hash. Run-time pipeline catalog still loads
				-- definitions directly from cartridge files; this table
				-- exists so callers (Studio, REST API, future runtime
				-- features) can reference a pipeline by stable id and
				-- track its version history without walking the cartridge
				-- tree.
				CREATE TABLE pipelines (
					id             UUID         PRIMARY KEY,
					cartridge_ref  VARCHAR(128) NOT NULL,
					name           VARCHAR(255) NOT NULL,
					version        INTEGER      NOT NULL DEFAULT 1,
					content_hash   CHAR(64)     NOT NULL,
					source_path    TEXT         NOT NULL,
					is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
					removed_at     TIMESTAMPTZ,
					meta           JSONB        NOT NULL DEFAULT '{}'::jsonb,
					created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_pipelines_natural UNIQUE (cartridge_ref, name)
				);
				CREATE INDEX ix_pipelines_active ON pipelines(is_active)
					WHERE is_active = TRUE;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS pipelines;
			`)
			return err
		},
	)
}

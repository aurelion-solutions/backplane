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
				-- Vocabulary tables, admin-managed.
				CREATE TABLE capabilities (
					id          UUID         PRIMARY KEY,
					slug        VARCHAR(128) NOT NULL UNIQUE,
					name        VARCHAR(255) NOT NULL,
					description TEXT,
					is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
					created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
				);

				CREATE TABLE capability_scope_keys (
					id          UUID         PRIMARY KEY,
					code        VARCHAR(64)  NOT NULL UNIQUE,
					name        VARCHAR(255) NOT NULL,
					description TEXT,
					is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
					created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
				);

				-- Mapping rules: per (capability, resource selector) what to project.
				-- resource selector is XOR over resource_id / resource_kind / resource_path_glob.
				CREATE TABLE capability_mappings (
					id                  UUID         PRIMARY KEY,
					capability_id       UUID         NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
					application_id      UUID         REFERENCES applications(id) ON DELETE CASCADE,
					resource_id         UUID,
					resource_kind       VARCHAR(128),
					resource_path_glob  VARCHAR(512),
					action_slug         VARCHAR(64),
					scope_key_id        UUID         NOT NULL REFERENCES capability_scope_keys(id),
					scope_value_source  JSONB        NOT NULL,
					is_active           BOOLEAN      NOT NULL DEFAULT TRUE,
					created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT ck_capability_mappings_resource_xor CHECK (
						(CASE WHEN resource_id        IS NOT NULL THEN 1 ELSE 0 END +
						 CASE WHEN resource_kind      IS NOT NULL THEN 1 ELSE 0 END +
						 CASE WHEN resource_path_glob IS NOT NULL THEN 1 ELSE 0 END) = 1
					)
				);
				CREATE INDEX ix_capability_mappings_capability  ON capability_mappings(capability_id);
				CREATE INDEX ix_capability_mappings_application ON capability_mappings(application_id);
				CREATE INDEX ix_capability_mappings_active      ON capability_mappings(is_active) WHERE is_active = TRUE;

				-- Output of projection. account_id, not principal_id — principals
				-- are attached by a separate matcher after normalize.
				CREATE TABLE capability_grants (
					id                            UUID         PRIMARY KEY,
					account_id                    UUID         NOT NULL REFERENCES accounts(id)            ON DELETE CASCADE,
					capability_id                 UUID         NOT NULL REFERENCES capabilities(id)        ON DELETE CASCADE,
					scope_key_id                  UUID         NOT NULL REFERENCES capability_scope_keys(id),
					scope_value                   VARCHAR(255),
					application_id                UUID         NOT NULL REFERENCES applications(id)        ON DELETE CASCADE,
					source_grant_external_id      VARCHAR(255) NOT NULL,
					source_capability_mapping_id  UUID         NOT NULL REFERENCES capability_mappings(id) ON DELETE CASCADE,
					observed_at                   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					tombstoned_at                 TIMESTAMPTZ,
					created_at                    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at                    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
				);
				CREATE UNIQUE INDEX uq_capability_grants_lineage
					ON capability_grants(source_grant_external_id, source_capability_mapping_id);
				CREATE INDEX ix_capability_grants_account    ON capability_grants(account_id);
				CREATE INDEX ix_capability_grants_capability ON capability_grants(capability_id);
				CREATE INDEX ix_capability_grants_natural    ON capability_grants(account_id, capability_id, scope_key_id, scope_value);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS capability_grants;
				DROP TABLE IF EXISTS capability_mappings;
				DROP TABLE IF EXISTS capability_scope_keys;
				DROP TABLE IF EXISTS capabilities;
			`)
			return err
		},
	)
}

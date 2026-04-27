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
				-- Determinator-resolution rules: per (provider, record_key) what
				-- person_key it maps to and whether the provider may CREATE new
				-- Persons (is_determinator) or only ATTACH to existing ones
				-- (allow_upstream).
				CREATE TABLE employee_provider_mappings (
					id              UUID         PRIMARY KEY,
					provider        VARCHAR(64)  NOT NULL,
					record_key      VARCHAR(128) NOT NULL,
					person_key      VARCHAR(128) NOT NULL,
					is_determinator BOOLEAN      NOT NULL DEFAULT FALSE,
					allow_upstream  BOOLEAN      NOT NULL DEFAULT FALSE,
					is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
					created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
				);
				CREATE UNIQUE INDEX uq_employee_provider_mappings_record
					ON employee_provider_mappings(provider, record_key);
				CREATE INDEX ix_employee_provider_mappings_active
					ON employee_provider_mappings(provider) WHERE is_active = TRUE;

				-- Lineage trail: which Employment is associated with which raw
				-- source record AND which period within it. One source record
				-- may carry several employment periods inline (see the
				-- dataset_type=employee contract); each gets its own match
				-- row, discriminated by period_start_date.
				CREATE TABLE employment_record_matches (
					id                          UUID         PRIMARY KEY,
					employment_id               UUID         NOT NULL REFERENCES employments(id) ON DELETE CASCADE,
					source                      VARCHAR(64)  NOT NULL,
					source_record_external_id   VARCHAR(255) NOT NULL,
					period_start_date           DATE         NOT NULL,
					matched_via_determinator    BOOLEAN      NOT NULL DEFAULT FALSE,
					created_at                  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at                  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
				);
				CREATE UNIQUE INDEX uq_employment_record_matches_lineage
					ON employment_record_matches(source, source_record_external_id, period_start_date);
				CREATE INDEX ix_employment_record_matches_employment
					ON employment_record_matches(employment_id);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS employment_record_matches;
				DROP TABLE IF EXISTS employee_provider_mappings;
			`)
			return err
		},
	)
}

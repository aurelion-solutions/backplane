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
				CREATE TABLE employments (
					id           UUID         PRIMARY KEY,
					person_id    UUID         NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
					code         VARCHAR(64)  NOT NULL,
					start_date   DATE         NOT NULL,
					end_date     DATE,
					org_unit_id  UUID         REFERENCES org_units(id) ON DELETE SET NULL,
					description  TEXT,
					created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_employments_person_code_start UNIQUE (person_id, code, start_date),
					CONSTRAINT ck_employments_end_after_start    CHECK (end_date IS NULL OR end_date >= start_date),
					CONSTRAINT ck_employments_code_not_empty     CHECK (length(btrim(code)) > 0)
				);
				CREATE INDEX ix_employments_person_id  ON employments(person_id);
				CREATE INDEX ix_employments_org_unit_id ON employments(org_unit_id);
				CREATE INDEX ix_employments_code        ON employments(code);
				CREATE UNIQUE INDEX uq_employments_person_active
					ON employments(person_id, code) WHERE end_date IS NULL;

				CREATE TABLE employment_attributes (
					id             UUID         PRIMARY KEY,
					employment_id  UUID         NOT NULL REFERENCES employments(id) ON DELETE CASCADE,
					key            VARCHAR(128) NOT NULL,
					value          TEXT         NOT NULL,
					CONSTRAINT uq_employment_attributes_emp_key UNIQUE (employment_id, key)
				);
				CREATE INDEX ix_employment_attributes_employment_id ON employment_attributes(employment_id);
				CREATE INDEX ix_employment_attributes_key_value     ON employment_attributes(key, value);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS employment_attributes;
				DROP TABLE IF EXISTS employments;
			`)
			return err
		},
	)
}

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
				CREATE TABLE employee_records (
					id              UUID         PRIMARY KEY,
					external_id     VARCHAR(255) NOT NULL,
					application_id  UUID         NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
					description     TEXT,
					CONSTRAINT uq_employee_records_app_external UNIQUE (application_id, external_id)
				);
				CREATE INDEX ix_employee_records_application_id ON employee_records(application_id);
				CREATE INDEX ix_employee_records_external_id    ON employee_records(external_id);

				CREATE TABLE employee_record_attributes (
					id                  UUID         PRIMARY KEY,
					employee_record_id  UUID         NOT NULL REFERENCES employee_records(id) ON DELETE CASCADE,
					key                 VARCHAR(128) NOT NULL,
					value               TEXT         NOT NULL,
					CONSTRAINT uq_employee_record_attributes_record_key UNIQUE (employee_record_id, key)
				);
				CREATE INDEX ix_employee_record_attributes_record_id ON employee_record_attributes(employee_record_id);
				CREATE INDEX ix_employee_record_attributes_key_value ON employee_record_attributes(key, value);

				CREATE TABLE employee_provider_attribute_mappings (
					id                   UUID         PRIMARY KEY,
					application_id       UUID         NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
					employee_record_key  VARCHAR(128) NOT NULL,
					person_key           VARCHAR(128) NOT NULL,
					is_determinator      BOOLEAN      NOT NULL DEFAULT FALSE,
					allow_upstream       BOOLEAN      NOT NULL DEFAULT FALSE,
					CONSTRAINT uq_epam_app_record_person UNIQUE (application_id, employee_record_key, person_key)
				);
				CREATE INDEX ix_epam_application_id  ON employee_provider_attribute_mappings(application_id);
				CREATE INDEX ix_epam_is_determinator ON employee_provider_attribute_mappings(is_determinator);

				CREATE TABLE employee_record_matches (
					id                        UUID    PRIMARY KEY,
					employee_record_id        UUID    NOT NULL REFERENCES employee_records(id) ON DELETE CASCADE,
					person_id                 UUID    NOT NULL REFERENCES persons(id)          ON DELETE CASCADE,
					employment_id             UUID    NOT NULL REFERENCES employments(id)      ON DELETE CASCADE,
					matched_via_determinator  BOOLEAN NOT NULL DEFAULT FALSE,
					CONSTRAINT uq_employee_record_matches_record UNIQUE (employee_record_id)
				);
				CREATE INDEX ix_employee_record_matches_person_id     ON employee_record_matches(person_id);
				CREATE INDEX ix_employee_record_matches_employment_id ON employee_record_matches(employment_id);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS employee_record_matches;
				DROP TABLE IF EXISTS employee_provider_attribute_mappings;
				DROP TABLE IF EXISTS employee_record_attributes;
				DROP TABLE IF EXISTS employee_records;
			`)
			return err
		},
	)
}

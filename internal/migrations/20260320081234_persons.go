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
				CREATE TABLE persons (
					id           UUID         PRIMARY KEY,
					external_id  VARCHAR(255) NOT NULL,
					full_name    VARCHAR(255) NOT NULL,
					CONSTRAINT uq_persons_external_id UNIQUE (external_id)
				);
				CREATE INDEX ix_persons_external_id ON persons(external_id);

				CREATE TABLE person_attributes (
					id         UUID         PRIMARY KEY,
					person_id  UUID         NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
					key        VARCHAR(128) NOT NULL,
					value      TEXT         NOT NULL,
					CONSTRAINT uq_person_attributes_person_key UNIQUE (person_id, key)
				);
				CREATE INDEX ix_person_attributes_person_id ON person_attributes(person_id);
				CREATE INDEX ix_person_attributes_key_value ON person_attributes(key, value);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS person_attributes;
				DROP TABLE IF EXISTS persons;
			`)
			return err
		},
	)
}

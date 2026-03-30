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
				CREATE TABLE principals (
					id                       UUID         PRIMARY KEY,
					external_id              VARCHAR(255) NOT NULL,
					kind                     VARCHAR(32)  NOT NULL,
					principal_employment_id  UUID         REFERENCES employments(id) ON DELETE CASCADE,
					principal_workload_id    UUID         REFERENCES workloads(id)   ON DELETE CASCADE,
					principal_customer_id    UUID         REFERENCES customers(id)   ON DELETE CASCADE,
					status                   VARCHAR(64)  NOT NULL,
					is_locked                BOOLEAN      NOT NULL DEFAULT FALSE,
					created_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_principals_external_id UNIQUE (external_id),
					CONSTRAINT ck_principals_kind CHECK (kind IN ('employment','workload','customer')),
					CONSTRAINT ck_principals_employment_body CHECK (
						kind <> 'employment' OR (
							principal_employment_id IS NOT NULL
							AND principal_workload_id  IS NULL
							AND principal_customer_id  IS NULL
						)
					),
					CONSTRAINT ck_principals_workload_body CHECK (
						kind <> 'workload' OR (
							principal_workload_id   IS NOT NULL
							AND principal_employment_id IS NULL
							AND principal_customer_id   IS NULL
						)
					),
					CONSTRAINT ck_principals_customer_body CHECK (
						kind <> 'customer' OR (
							principal_customer_id   IS NOT NULL
							AND principal_employment_id IS NULL
							AND principal_workload_id   IS NULL
						)
					)
				);
				CREATE UNIQUE INDEX uq_principals_employment_body
					ON principals(principal_employment_id) WHERE principal_employment_id IS NOT NULL;
				CREATE UNIQUE INDEX uq_principals_workload_body
					ON principals(principal_workload_id)   WHERE principal_workload_id   IS NOT NULL;
				CREATE UNIQUE INDEX uq_principals_customer_body
					ON principals(principal_customer_id)   WHERE principal_customer_id   IS NOT NULL;
				CREATE INDEX ix_principals_kind_locked ON principals(kind, is_locked);

				CREATE TABLE principal_attributes (
					id            UUID         PRIMARY KEY,
					principal_id  UUID         NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
					key           VARCHAR(128) NOT NULL,
					value         TEXT         NOT NULL,
					CONSTRAINT uq_principal_attributes_principal_key UNIQUE (principal_id, key)
				);
				CREATE INDEX ix_principal_attributes_principal_id ON principal_attributes(principal_id);
				CREATE INDEX ix_principal_attributes_key_value    ON principal_attributes(key, value);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS principal_attributes;
				DROP TABLE IF EXISTS principals;
			`)
			return err
		},
	)
}

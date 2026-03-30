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
				CREATE TABLE workloads (
					id                   UUID         PRIMARY KEY,
					external_id          VARCHAR(255) NOT NULL,
					name                 VARCHAR(255) NOT NULL,
					description          TEXT,
					owner_employment_id  UUID         REFERENCES employments(id) ON DELETE SET NULL,
					application_id       UUID         REFERENCES applications(id) ON DELETE SET NULL,
					CONSTRAINT uq_workloads_external_id UNIQUE (external_id)
				);
				CREATE INDEX ix_workloads_external_id         ON workloads(external_id);
				CREATE INDEX ix_workloads_owner_employment_id ON workloads(owner_employment_id);
				CREATE INDEX ix_workloads_application_id      ON workloads(application_id);

				CREATE TABLE workload_attributes (
					id           UUID         PRIMARY KEY,
					workload_id  UUID         NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
					key          VARCHAR(128) NOT NULL,
					value        TEXT         NOT NULL,
					CONSTRAINT uq_workload_attributes_workload_key UNIQUE (workload_id, key)
				);
				CREATE INDEX ix_workload_attributes_workload_id ON workload_attributes(workload_id);
				CREATE INDEX ix_workload_attributes_key_value   ON workload_attributes(key, value);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS workload_attributes;
				DROP TABLE IF EXISTS workloads;
			`)
			return err
		},
	)
}

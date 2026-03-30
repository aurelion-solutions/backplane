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
				CREATE TABLE customers (
					id              UUID         PRIMARY KEY,
					external_id     VARCHAR(255) NOT NULL,
					email_verified  BOOLEAN      NOT NULL DEFAULT FALSE,
					tenant_id       VARCHAR(255),
					tenant_role     VARCHAR(32),
					plan_tier       VARCHAR(32),
					mfa_enabled     BOOLEAN      NOT NULL DEFAULT FALSE,
					description     TEXT,
					created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_customers_external_id UNIQUE (external_id),
					CONSTRAINT ck_customers_tenant_role CHECK (
						tenant_role IS NULL OR tenant_role IN ('owner','admin','member','viewer')
					),
					CONSTRAINT ck_customers_plan_tier CHECK (
						plan_tier IS NULL OR plan_tier IN ('free','starter','pro','enterprise')
					)
				);
				CREATE INDEX ix_customers_external_id ON customers(external_id);
				CREATE INDEX ix_customers_tenant_id   ON customers(tenant_id);

				CREATE TABLE customer_attributes (
					id           UUID         PRIMARY KEY,
					customer_id  UUID         NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
					key          VARCHAR(128) NOT NULL,
					value        TEXT         NOT NULL,
					created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_customer_attributes_customer_key UNIQUE (customer_id, key)
				);
				CREATE INDEX ix_customer_attributes_customer_id ON customer_attributes(customer_id);
				CREATE INDEX ix_customer_attributes_key_value   ON customer_attributes(key, value);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS customer_attributes;
				DROP TABLE IF EXISTS customers;
			`)
			return err
		},
	)
}

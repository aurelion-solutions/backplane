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
			// Secret findings record an evidence chain whose normalized
			// fact is the secret row itself. Widen the normalized_kind
			// CHECK to admit the two secret entities.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE evidence_chains
					DROP CONSTRAINT IF EXISTS evidence_chains_normalized_kind_check;
				ALTER TABLE evidence_chains
					ADD CONSTRAINT evidence_chains_normalized_kind_check
					CHECK (normalized_kind IS NULL OR normalized_kind IN
						('person','employment','account','workload',
						 'secret_plain','secret_certificate'));
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE evidence_chains
					DROP CONSTRAINT IF EXISTS evidence_chains_normalized_kind_check;
				ALTER TABLE evidence_chains
					ADD CONSTRAINT evidence_chains_normalized_kind_check
					CHECK (normalized_kind IS NULL OR normalized_kind IN
						('person','employment','account','workload'));
			`)
			return err
		},
	)
}

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
			// Slice 2b: PEO must hold non-entity targets (source,
			// pipeline) for aggregate coverage gaps. target_ref is no
			// longer always a uuid, so make it nullable and move row
			// identity onto target_key (always present: username for
			// accounts, source name / pipeline stage for aggregates).
			_, err := db.ExecContext(ctx, `
				ALTER TABLE policy_evaluation_outcomes
					ALTER COLUMN target_ref DROP NOT NULL;

				ALTER TABLE policy_evaluation_outcomes
					DROP CONSTRAINT IF EXISTS policy_evaluation_outcomes_target_type_check;
				ALTER TABLE policy_evaluation_outcomes
					ADD CONSTRAINT policy_evaluation_outcomes_target_type_check
					CHECK (target_type IN ('account','subject','nhi','source','pipeline'));

				DROP INDEX IF EXISTS uq_peo_identity;
				CREATE UNIQUE INDEX uq_peo_identity
					ON policy_evaluation_outcomes
					(assessment_run_id, cartridge_id, rule_id, target_type, target_key)
					NULLS NOT DISTINCT;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP INDEX IF EXISTS uq_peo_identity;
				CREATE UNIQUE INDEX uq_peo_identity
					ON policy_evaluation_outcomes
					(assessment_run_id, cartridge_id, rule_id, target_type, target_ref)
					NULLS NOT DISTINCT;

				ALTER TABLE policy_evaluation_outcomes
					DROP CONSTRAINT IF EXISTS policy_evaluation_outcomes_target_type_check;
				ALTER TABLE policy_evaluation_outcomes
					ADD CONSTRAINT policy_evaluation_outcomes_target_type_check
					CHECK (target_type IN ('account','subject','nhi'));
			`)
			return err
		},
	)
}

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
			// policy_evaluation_outcomes (PEO) — the ternary record for
			// every (policy, target) evaluation in a run. findings stays
			// the matched-violation set; PEO is the superset that also
			// records not_matched and not_evaluable (Blind Spots).
			//
			// Biconditional invariant (outcome = not_evaluable ⇔
			// missing_evidence non-empty) is enforced in the service and
			// asserted in tests; the CHECK here pins the closed sets.
			_, err := db.ExecContext(ctx, `
				CREATE TABLE policy_evaluation_outcomes (
					id                 UUID PRIMARY KEY,
					assessment_run_id  UUID NOT NULL REFERENCES policy_assessment_runs(id) ON DELETE RESTRICT,
					cartridge_id       TEXT NOT NULL,
					rule_id            TEXT NOT NULL,
					target_type        TEXT NOT NULL CHECK (target_type IN ('account','subject','nhi')),
					target_ref         UUID NOT NULL,
					target_key         TEXT NOT NULL,
					outcome            TEXT NOT NULL CHECK (outcome IN ('matched','not_matched','not_evaluable')),
					missing_evidence   JSONB NOT NULL DEFAULT '[]',
					source_id          UUID NULL,
					evaluated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				-- Re-emission within the same run upserts rather than
				-- duplicating. NULLS NOT DISTINCT so the unique key is
				-- stable even when a nullable column participates later.
				CREATE UNIQUE INDEX uq_peo_identity
					ON policy_evaluation_outcomes
					(assessment_run_id, cartridge_id, rule_id, target_type, target_ref)
					NULLS NOT DISTINCT;

				CREATE INDEX ix_peo_run_outcome
					ON policy_evaluation_outcomes (assessment_run_id, outcome);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS policy_evaluation_outcomes;
			`)
			return err
		},
	)
}

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
			// Rename the posture-domain "nhi" vocabulary to "workload":
			// PEO target_type, the CHECK that pins it, and the finding
			// kinds emitted by the workload-posture and core-identity
			// cartridges. Drop the CHECK first so the UPDATE is legal,
			// then re-add it with the new closed set.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE policy_evaluation_outcomes
					DROP CONSTRAINT IF EXISTS policy_evaluation_outcomes_target_type_check;
				UPDATE policy_evaluation_outcomes SET target_type = 'workload' WHERE target_type = 'nhi';
				ALTER TABLE policy_evaluation_outcomes
					ADD CONSTRAINT policy_evaluation_outcomes_target_type_check
					CHECK (target_type IN ('account','subject','workload','source','pipeline'));

				UPDATE findings SET kind = 'workload_owned_by_terminated' WHERE kind = 'nhi_owned_by_terminated';
				UPDATE findings SET kind = 'workload_unowned' WHERE kind = 'nhi_unowned';
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				UPDATE findings SET kind = 'nhi_unowned' WHERE kind = 'workload_unowned';
				UPDATE findings SET kind = 'nhi_owned_by_terminated' WHERE kind = 'workload_owned_by_terminated';

				ALTER TABLE policy_evaluation_outcomes
					DROP CONSTRAINT IF EXISTS policy_evaluation_outcomes_target_type_check;
				UPDATE policy_evaluation_outcomes SET target_type = 'nhi' WHERE target_type = 'workload';
				ALTER TABLE policy_evaluation_outcomes
					ADD CONSTRAINT policy_evaluation_outcomes_target_type_check
					CHECK (target_type IN ('account','subject','nhi','source','pipeline'));
			`)
			return err
		},
	)
}

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
			// A finding explanation is a generated narrative over an
			// already-proven finding — a persisted, cited artifact, not a
			// view. It carries no authority of its own: the finding,
			// severity, evidence and policy are decided upstream; this row
			// only packages them into prose plus the validated citations
			// back to the input refs.
			//
			// Cached by (finding_id, input_hash): input_hash digests the
			// rendered prompt inputs (finding + evidence refs + policy +
			// template version + model). The same inputs never regenerate;
			// a change in any of them yields a new hash. A failed row is a
			// generation failure, never a finding failure.
			_, err := db.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS finding_explanations (
					id                      uuid PRIMARY KEY,
					finding_id              uuid NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
					assessment_run_id       uuid NOT NULL,
					policy_id               uuid,
					input_hash              text NOT NULL,
					model_ref               text NOT NULL,
					prompt_template_version text NOT NULL,
					status                  text NOT NULL,
					narrative               text NOT NULL DEFAULT '',
					citations               jsonb NOT NULL DEFAULT '[]'::jsonb,
					refs                    jsonb NOT NULL DEFAULT '[]'::jsonb,
					error                   text,
					created_at              timestamptz NOT NULL,
					completed_at            timestamptz,
					CONSTRAINT finding_explanations_status_check
						CHECK (status IN ('pending','running','completed','failed')),
					CONSTRAINT finding_explanations_finding_hash_key
						UNIQUE (finding_id, input_hash)
				)
			`)
			if err != nil {
				return err
			}
			if _, err := db.ExecContext(ctx, `
				CREATE INDEX IF NOT EXISTS finding_explanations_finding_idx
					ON finding_explanations (finding_id)
			`); err != nil {
				return err
			}
			_, err = db.ExecContext(ctx, `
				CREATE INDEX IF NOT EXISTS finding_explanations_run_idx
					ON finding_explanations (assessment_run_id)
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS finding_explanations`)
			return err
		},
	)
}

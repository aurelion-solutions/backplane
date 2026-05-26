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
			// A finding carries two independent axes:
			//   - principal_id: the identity it concerns (the "who" —
			//     the account's owner or the workload's own principal),
			//   - target_type + target_id: the concrete artifact the
			//     problem sits on (account | workload | secret_plain |
			//     secret_certificate).
			//
			// They are NOT alternatives. The old model encoded the artifact
			// as account_id XOR principal_id, which both overloaded
			// principal_id (it held the workload's principal as a target)
			// and left account findings' owner principal unset. This
			// migration promotes the evidence_chain normalized-kind idea
			// onto the finding row as a discriminated target, fixes the
			// account owner linkage, and retires account_id.
			//
			// target_id is a polymorphic reference (no FK — it points at
			// one of four tables). Findings are run-scoped snapshots tracked
			// by last_seen_run_id, so a dangling target_id after the
			// artifact is deleted is acceptable; the identity axis
			// (principal_id) keeps its FK.
			_, err := db.ExecContext(ctx, `
				ALTER TABLE findings
					ADD COLUMN IF NOT EXISTS target_type text,
					ADD COLUMN IF NOT EXISTS target_id   uuid;

				-- Account findings: artifact = the account.
				UPDATE findings
					SET target_type = 'account', target_id = account_id
					WHERE account_id IS NOT NULL;

				-- Fix the identity axis: an account finding's principal is
				-- the account's owner (was left NULL before).
				UPDATE findings f
					SET principal_id = a.principal_id
					FROM accounts a
					WHERE f.account_id = a.id
						AND f.principal_id IS NULL
						AND a.principal_id IS NOT NULL;

				-- Workload findings (principal set, no account): artifact =
				-- the workload body behind that principal. principal_id stays.
				UPDATE findings f
					SET target_type = 'workload', target_id = p.principal_workload_id
					FROM principals p
					WHERE f.principal_id = p.id
						AND f.account_id IS NULL
						AND p.kind = 'workload'
						AND p.principal_workload_id IS NOT NULL;

				ALTER TABLE findings DROP COLUMN IF EXISTS account_id;

				ALTER TABLE findings
					ADD CONSTRAINT findings_target_type_check
					CHECK (target_type IS NULL OR target_type IN
						('account','workload','secret_plain','secret_certificate'));

				CREATE INDEX IF NOT EXISTS idx_findings_target
					ON findings (target_type, target_id) WHERE target_id IS NOT NULL;

				-- Dropping account_id also dropped the two constraints that
				-- referenced it: the idempotency unique key and the
				-- "must anchor to identity or artifact" check. Recreate both
				-- over the new axes. The unique key is what makes a re-run
				-- re-confirm (duplicate-key) instead of inserting a twin.
				ALTER TABLE findings
					ADD CONSTRAINT ck_findings_principal_or_target
					CHECK (principal_id IS NOT NULL OR target_id IS NOT NULL);
				ALTER TABLE findings
					ADD CONSTRAINT uq_findings_evidence UNIQUE NULLS NOT DISTINCT (
						kind, principal_id, target_type, target_id,
						policy_id, scope_key_id, scope_value, evidence_hash
					);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE findings DROP CONSTRAINT IF EXISTS uq_findings_evidence;
				ALTER TABLE findings DROP CONSTRAINT IF EXISTS ck_findings_principal_or_target;
				ALTER TABLE findings ADD COLUMN IF NOT EXISTS account_id uuid;
				UPDATE findings SET account_id = target_id WHERE target_type = 'account';
				DROP INDEX IF EXISTS idx_findings_target;
				ALTER TABLE findings DROP CONSTRAINT IF EXISTS findings_target_type_check;
				ALTER TABLE findings
					DROP COLUMN IF EXISTS target_type,
					DROP COLUMN IF EXISTS target_id;
				ALTER TABLE findings
					ADD CONSTRAINT ck_findings_principal_or_account
					CHECK (principal_id IS NOT NULL OR account_id IS NOT NULL);
				ALTER TABLE findings
					ADD CONSTRAINT uq_findings_evidence UNIQUE NULLS NOT DISTINCT (
						kind, principal_id, account_id,
						policy_id, scope_key_id, scope_value, evidence_hash
					);
			`)
			return err
		},
	)
}

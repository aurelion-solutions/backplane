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
				-- Persistence layer for policy_assessment engine output.
				-- One policy_assessment_runs row per evaluation pass
				-- (worker policy-assessment action, ad-hoc REST trigger,
				-- scheduled cron); one findings row per matched policy
				-- output.
				CREATE TABLE policy_assessment_runs (
					id                       UUID         PRIMARY KEY,
					status                   VARCHAR(32)  NOT NULL DEFAULT 'pending',
					triggered_by             VARCHAR(32)  NOT NULL,
					started_at               TIMESTAMPTZ,
					completed_at             TIMESTAMPTZ,
					scope_principal_id       UUID         REFERENCES principals(id)   ON DELETE RESTRICT,
					scope_application_id     UUID         REFERENCES applications(id) ON DELETE RESTRICT,
					findings_total           INTEGER      NOT NULL DEFAULT 0,
					findings_by_severity     JSONB        NOT NULL DEFAULT '{}'::jsonb,
					findings_created_count   INTEGER      NOT NULL DEFAULT 0,
					findings_reused_count    INTEGER      NOT NULL DEFAULT 0,
					error_message            TEXT,
					created_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					created_by               VARCHAR(255),
					CONSTRAINT ck_par_status_value             CHECK (status IN ('pending','running','completed','failed')),
					CONSTRAINT ck_par_trigger_value            CHECK (triggered_by IN ('manual','api','schedule')),
					CONSTRAINT ck_par_completed_at_terminal    CHECK (completed_at IS NULL OR status IN ('completed','failed')),
					CONSTRAINT ck_par_started_at_not_pending   CHECK (started_at IS NOT NULL OR status = 'pending'),
					CONSTRAINT ck_par_findings_total_nonneg    CHECK (findings_total           >= 0),
					CONSTRAINT ck_par_findings_created_nonneg  CHECK (findings_created_count   >= 0),
					CONSTRAINT ck_par_findings_reused_nonneg   CHECK (findings_reused_count    >= 0)
				);
				CREATE INDEX ix_par_status               ON policy_assessment_runs(status);
				CREATE INDEX ix_par_created_at_desc      ON policy_assessment_runs(created_at DESC);
				CREATE INDEX ix_par_scope_principal_id   ON policy_assessment_runs(scope_principal_id);
				CREATE INDEX ix_par_scope_application_id ON policy_assessment_runs(scope_application_id);

				-- One row per detected violation / anomaly. Kind is a free
				-- VARCHAR — the vocabulary of anomaly kinds is owned by
				-- the policy that emits each finding. New kinds arrive
				-- through policy manifests without touching this schema.
				--
				-- At least one of principal_id / account_id must be set
				-- (CHECK). active_mitigation_id / proposed_mitigation_id
				-- are plain UUID columns without FK — the constraint will
				-- be added when the mitigations slice ships.
				CREATE TABLE findings (
					id                              UUID         PRIMARY KEY,
					assessment_run_id               UUID         NOT NULL REFERENCES policy_assessment_runs(id) ON DELETE RESTRICT,
					kind                            VARCHAR(64)  NOT NULL,
					principal_id                    UUID         REFERENCES principals(id)             ON DELETE RESTRICT,
					account_id                      UUID         REFERENCES accounts(id)               ON DELETE RESTRICT,
					policy_id                       UUID         REFERENCES policies(id)               ON DELETE RESTRICT,
					scope_key_id                    UUID         REFERENCES capability_scope_keys(id)  ON DELETE RESTRICT,
					scope_value                     VARCHAR(255),
					severity                        VARCHAR(32)  NOT NULL,
					status                          VARCHAR(32)  NOT NULL DEFAULT 'open',
					matched_capability_grant_ids    JSONB        NOT NULL DEFAULT '[]'::jsonb,
					matched_effective_grant_ids     JSONB        NOT NULL DEFAULT '[]'::jsonb,
					matched_access_fact_ids         JSONB        NOT NULL DEFAULT '[]'::jsonb,
					evidence_hash                   VARCHAR(64)  NOT NULL,
					active_mitigation_id            UUID,
					proposed_mitigation_id          UUID,
					detected_at                     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
					evaluated_at                    TIMESTAMPTZ  NOT NULL,
					status_changed_at               TIMESTAMPTZ,
					status_reason                   TEXT,
					CONSTRAINT ck_findings_status_value             CHECK (status   IN ('open','acknowledged','resolved','mitigated')),
					CONSTRAINT ck_findings_severity_value           CHECK (severity IN ('critical','high','medium','low')),
					CONSTRAINT ck_findings_principal_or_account     CHECK (principal_id IS NOT NULL OR account_id IS NOT NULL),
					CONSTRAINT uq_findings_evidence UNIQUE NULLS NOT DISTINCT (
						kind, principal_id, account_id, policy_id, scope_key_id, scope_value, evidence_hash
					)
				);
				CREATE INDEX ix_findings_principal_status       ON findings(principal_id, status);
				CREATE INDEX ix_findings_policy_status          ON findings(policy_id, status);
				CREATE INDEX ix_findings_kind_status_detected   ON findings(kind, status, detected_at DESC);
				CREATE INDEX ix_findings_severity_status        ON findings(severity, status);
				CREATE INDEX ix_findings_active_mitigation_id   ON findings(active_mitigation_id);
				CREATE INDEX ix_findings_proposed_mitigation_id ON findings(proposed_mitigation_id);
				CREATE INDEX ix_findings_assessment_run_id      ON findings(assessment_run_id);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS findings;
				DROP TABLE IF EXISTS policy_assessment_runs;
			`)
			return err
		},
	)
}

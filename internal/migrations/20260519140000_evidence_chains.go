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
			// evidence_chains — append-only lineage from an outcome /
			// finding back through the truth stack to the raw row. Never
			// updated in place; idempotent on chain_hash. Anchored to a
			// scan run so evidence and outcomes share an immutable run
			// reference (the temporal foundation for Slice 5 period
			// queries). All lineage FKs nullable: a chain records
			// whichever layers exist.
			_, err := db.ExecContext(ctx, `
				CREATE TABLE evidence_chains (
					id                  UUID PRIMARY KEY,
					scan_run_id         UUID NOT NULL REFERENCES policy_assessment_runs(id) ON DELETE RESTRICT,
					finding_id          UUID NULL REFERENCES findings(id) ON DELETE SET NULL,
					outcome_id          UUID NULL REFERENCES policy_evaluation_outcomes(id) ON DELETE SET NULL,
					ingest_batch_id     UUID NULL,
					raw_row_hash        TEXT NULL,
					normalized_kind     TEXT NULL CHECK (normalized_kind IN ('person','employment','account','workload')),
					normalized_id       UUID NULL,
					capability_grant_id UUID NULL,
					initiative_id       UUID NULL,
					policy_ref          TEXT NOT NULL DEFAULT '',
					chain_hash          TEXT NOT NULL,
					created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE UNIQUE INDEX uq_evidence_chains_hash ON evidence_chains (chain_hash);
				CREATE INDEX ix_evidence_chains_finding ON evidence_chains (finding_id);
				CREATE INDEX ix_evidence_chains_run     ON evidence_chains (scan_run_id);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS evidence_chains;
			`)
			return err
		},
	)
}

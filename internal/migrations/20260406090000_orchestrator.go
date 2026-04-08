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
				-- ----------------------------------------------------------
				-- Status enums
				-- ----------------------------------------------------------

				CREATE TYPE pipeline_run_status AS ENUM (
					'pending',
					'running',
					'awaiting_event',
					'cancelling',
					'completed',
					'failed',
					'failed_timeout',
					'cancelled'
				);

				CREATE TYPE step_run_status AS ENUM (
					'pending',
					'running',
					'awaiting_event',
					'completed',
					'failed',
					'failed_timeout',
					'aborted',
					'cancelled'
				);

				CREATE TYPE pipeline_event_waiter_status AS ENUM (
					'waiting',
					'matched',
					'expired',
					'cancelled'
				);

				CREATE TYPE pipeline_trigger_source AS ENUM (
					'http',
					'mq',
					'schedule',
					'retry'
				);

				-- ----------------------------------------------------------
				-- pipeline_runs
				-- ----------------------------------------------------------

				CREATE TABLE pipeline_runs (
					id                   UUID                       PRIMARY KEY DEFAULT gen_random_uuid(),
					pipeline_name        VARCHAR(255)               NOT NULL,
					pipeline_version     INTEGER                    NOT NULL,
					args                 JSONB                      NOT NULL DEFAULT '{}'::jsonb,
					content_hash         CHAR(64)                   NOT NULL,
					status               pipeline_run_status        NOT NULL DEFAULT 'pending',
					current_step         VARCHAR(255),
					retry_of_run_id      UUID                       REFERENCES pipeline_runs(id) ON DELETE SET NULL,
					trigger_source       pipeline_trigger_source    NOT NULL,
					started_at           TIMESTAMPTZ,
					finished_at          TIMESTAMPTZ,
					error                TEXT,
					worker_id            VARCHAR(255),
					last_heartbeat_at    TIMESTAMPTZ,
					created_at           TIMESTAMPTZ                NOT NULL DEFAULT NOW(),
					updated_at           TIMESTAMPTZ                NOT NULL DEFAULT NOW()
				);

				-- Work-acquisition index: SELECT ... FOR UPDATE SKIP LOCKED on
				-- (status='pending', stale-or-null heartbeat, ORDER BY created_at).
				CREATE INDEX ix_pipeline_runs_status_heartbeat_created
					ON pipeline_runs(status, last_heartbeat_at, created_at);

				-- Partial UNIQUE: blocks duplicate in-flight runs for the same
				-- (pipeline_name, pipeline_version, content_hash); retries
				-- (retry_of_run_id IS NOT NULL) and terminal rows bypass.
				CREATE UNIQUE INDEX uq_pipeline_runs_inflight_idempotency
					ON pipeline_runs(pipeline_name, pipeline_version, content_hash)
					WHERE retry_of_run_id IS NULL
					  AND status IN ('pending', 'running', 'awaiting_event', 'cancelling');

				-- ----------------------------------------------------------
				-- step_runs
				-- ----------------------------------------------------------

				CREATE TABLE step_runs (
					id                UUID              PRIMARY KEY DEFAULT gen_random_uuid(),
					pipeline_run_id   UUID              NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
					step_name         VARCHAR(255)      NOT NULL,
					attempt           INTEGER           NOT NULL DEFAULT 1,
					status            step_run_status   NOT NULL DEFAULT 'pending',
					args              JSONB             NOT NULL DEFAULT '{}'::jsonb,
					result            JSONB,
					error             TEXT,
					started_at        TIMESTAMPTZ,
					finished_at       TIMESTAMPTZ,
					created_at        TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
					updated_at        TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_step_runs_run_step_attempt UNIQUE (pipeline_run_id, step_name, attempt)
				);
				CREATE INDEX ix_step_runs_pipeline_run_id ON step_runs(pipeline_run_id);

				-- ----------------------------------------------------------
				-- pipeline_event_waiters
				-- ----------------------------------------------------------

				CREATE TABLE pipeline_event_waiters (
					id            UUID                            PRIMARY KEY DEFAULT gen_random_uuid(),
					step_run_id   UUID                            NOT NULL REFERENCES step_runs(id) ON DELETE CASCADE,
					event_type    VARCHAR(255)                    NOT NULL,
					match         JSONB                           NOT NULL DEFAULT '{}'::jsonb,
					expires_at    TIMESTAMPTZ                     NOT NULL,
					status        pipeline_event_waiter_status    NOT NULL DEFAULT 'waiting',
					created_at    TIMESTAMPTZ                     NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_pipeline_event_waiters_step_run_id UNIQUE (step_run_id)
				);
				CREATE INDEX ix_pipeline_event_waiters_event_type ON pipeline_event_waiters(event_type);
				CREATE INDEX ix_pipeline_event_waiters_expires_at ON pipeline_event_waiters(expires_at);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS pipeline_event_waiters;
				DROP TABLE IF EXISTS step_runs;
				DROP TABLE IF EXISTS pipeline_runs;
				DROP TYPE IF EXISTS pipeline_trigger_source;
				DROP TYPE IF EXISTS pipeline_event_waiter_status;
				DROP TYPE IF EXISTS step_run_status;
				DROP TYPE IF EXISTS pipeline_run_status;
			`)
			return err
		},
	)
}

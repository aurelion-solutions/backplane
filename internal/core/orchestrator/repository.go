// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for the three orchestrator
// tables. Every method takes a bun.IDB so the caller (Service)
// controls transaction scope — there is no single-shot connection
// hidden inside.
type Repository interface {
	// PipelineRun
	InsertRun(ctx context.Context, db bun.IDB, r *PipelineRun) error
	GetRun(ctx context.Context, db bun.IDB, id uuid.UUID) (*PipelineRun, error)
	ListRuns(ctx context.Context, db bun.IDB, filters ListRunsFilters) ([]*PipelineRun, int, error)
	ListRunsByPipeline(ctx context.Context, db bun.IDB, pipelineName string, limit, offset int) ([]*PipelineRun, int, error)
	FindInflightRun(ctx context.Context, db bun.IDB, pipelineName string, pipelineVersion int, contentHash string) (*PipelineRun, error)

	// Status-guarded UPDATEs on pipeline_runs. Return rowcount so the
	// service can branch on cancel-vs-complete races.
	UpdateRunStatusGuarded(ctx context.Context, db bun.IDB, id uuid.UUID, from []RunStatus, to RunStatus, fields RunUpdateFields) (int64, error)

	// ClaimPendingRun runs the SELECT … FOR UPDATE SKIP LOCKED +
	// guarded UPDATE in a single transaction. Returns the claimed row
	// (refreshed) or nil when nothing was claimable.
	ClaimPendingRun(ctx context.Context, db bun.IDB, workerID string, now time.Time) (*PipelineRun, error)

	// ScheduleAlreadyFired reports whether a schedule-triggered run
	// for the given (pipelineName, version) was created at or after
	// firePoint — used by Beat to dedupe within the cron window.
	ScheduleAlreadyFired(ctx context.Context, db bun.IDB, pipelineName string, pipelineVersion int, firePoint time.Time) (bool, error)

	// RefreshHeartbeat is the only writer method that does NOT change
	// status — pure liveness tick on (worker_id, status=running).
	RefreshHeartbeat(ctx context.Context, db bun.IDB, runID uuid.UUID, workerID string, now time.Time) (int64, error)

	// ReclaimStaleRun runs the lock + guarded UPDATE that releases a
	// stale run; returns (previousWorkerID, lastHeartbeatAt) for the
	// caller to fold into the heartbeat_lost event.
	ReclaimStaleRun(ctx context.Context, db bun.IDB, runID uuid.UUID, staleThreshold time.Duration) (workerID *string, lastHb *time.Time, ok bool, err error)
	ListStaleRunIDs(ctx context.Context, db bun.IDB, staleThreshold time.Duration, limit int) ([]uuid.UUID, error)

	// UpsertWorkerSlot writes the registry row for a live runner slot.
	// Called on slot startup and on every heartbeat tick.
	UpsertWorkerSlot(ctx context.Context, db bun.IDB, slot *WorkerSlot) error

	// DeleteWorkerSlot removes the registry row on graceful shutdown.
	DeleteWorkerSlot(ctx context.Context, db bun.IDB, workerID string) error

	// ListWorkers reads the worker_slots registry filtered by
	// heartbeat freshness and joins per-worker active-run aggregates
	// from pipeline_runs. Returns idle slots too — they are simply
	// rows with active_runs = 0.
	ListWorkers(ctx context.Context, db bun.IDB, staleThreshold time.Duration) ([]WorkerSummary, error)

	// ListRunsByWorker returns runs assigned to workerID. Used by the
	// worker detail panel; orders by started_at DESC NULLS LAST then
	// id DESC.
	ListRunsByWorker(ctx context.Context, db bun.IDB, workerID string, limit, offset int) ([]*PipelineRun, int, error)

	// StepRun
	InsertStep(ctx context.Context, db bun.IDB, s *StepRun) error
	GetStep(ctx context.Context, db bun.IDB, id uuid.UUID) (*StepRun, error)
	LatestStepAttempt(ctx context.Context, db bun.IDB, runID uuid.UUID, stepName string, forUpdate bool) (*StepRun, error)
	ListStepsByRun(ctx context.Context, db bun.IDB, runID uuid.UUID) ([]*StepRun, error)
	UpdateStepStatusGuarded(ctx context.Context, db bun.IDB, id uuid.UUID, from []StepStatus, to StepStatus, fields StepUpdateFields) (int64, error)
	UpdateRunCurrentStep(ctx context.Context, db bun.IDB, id uuid.UUID, stepName string) error
	AbortRunningStepsForRun(ctx context.Context, db bun.IDB, runID uuid.UUID) error

	// EventWaiter
	InsertWaiter(ctx context.Context, db bun.IDB, w *EventWaiter) error
	GetWaiterByStepRun(ctx context.Context, db bun.IDB, stepRunID uuid.UUID, forUpdate bool) (*EventWaiter, error)
	DeleteWaiterByStepRun(ctx context.Context, db bun.IDB, stepRunID uuid.UUID) error
	FindMatchingWaiterStepIDs(ctx context.Context, db bun.IDB, eventType string, payload map[string]any) ([]uuid.UUID, error)
	ListExpiredWaiterStepIDs(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]uuid.UUID, error)
}

// ListRunsFilters captures the GET /pipelines/runs query parameters.
type ListRunsFilters struct {
	Pipeline string
	Statuses []RunStatus
	Limit    int
	Offset   int
}

// RunUpdateFields carries the optional column writes that a
// status-guarded UPDATE can perform alongside the status transition.
type RunUpdateFields struct {
	WorkerID        *string
	ClearWorkerID   bool
	LastHeartbeatAt *time.Time
	ClearHeartbeat  bool
	StartedAt       *time.Time
	ClearStartedAt  bool
	FinishedAt      *time.Time
	ErrorMsg        *string
}

// StepUpdateFields is the step-level equivalent.
type StepUpdateFields struct {
	Result     map[string]any
	ErrorMsg   *string
	FinishedAt *time.Time
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct{}

// NewBunRepository constructs the bun-backed Repository.
func NewBunRepository() *BunRepository { return &BunRepository{} }

// -------------------------------------------------------------------
// PipelineRun
// -------------------------------------------------------------------

func (r *BunRepository) InsertRun(ctx context.Context, db bun.IDB, run *PipelineRun) error {
	_, err := db.NewInsert().Model(run).Returning("*").Exec(ctx)
	return err
}

func (r *BunRepository) GetRun(ctx context.Context, db bun.IDB, id uuid.UUID) (*PipelineRun, error) {
	out := new(PipelineRun)
	err := db.NewSelect().Model(out).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) ListRuns(ctx context.Context, db bun.IDB, f ListRunsFilters) ([]*PipelineRun, int, error) {
	q := db.NewSelect().Model((*PipelineRun)(nil))
	if f.Pipeline != "" {
		q = q.Where("pipeline_name = ?", f.Pipeline)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status IN (?)", bun.In(toStrings(f.Statuses)))
	}
	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := []*PipelineRun{}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	err = q.OrderExpr("started_at DESC NULLS LAST, id DESC").
		Limit(limit).Offset(f.Offset).Scan(ctx, &out)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *BunRepository) ListRunsByPipeline(ctx context.Context, db bun.IDB, name string, limit, offset int) ([]*PipelineRun, int, error) {
	return r.ListRuns(ctx, db, ListRunsFilters{Pipeline: name, Limit: limit, Offset: offset})
}

func (r *BunRepository) FindInflightRun(ctx context.Context, db bun.IDB, name string, version int, hash string) (*PipelineRun, error) {
	out := new(PipelineRun)
	err := db.NewSelect().Model(out).
		Where("pipeline_name = ?", name).
		Where("pipeline_version = ?", version).
		Where("content_hash = ?", hash).
		Where("retry_of_run_id IS NULL").
		Where("status IN (?)", bun.In([]string{
			string(RunPending), string(RunRunning), string(RunAwaitingEvent), string(RunCancelling),
		})).
		Limit(1).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) UpdateRunStatusGuarded(
	ctx context.Context, db bun.IDB, id uuid.UUID,
	from []RunStatus, to RunStatus, fields RunUpdateFields,
) (int64, error) {
	q := db.NewUpdate().Model((*PipelineRun)(nil)).
		Where("id = ?", id).
		Where("status IN (?)", bun.In(toStrings(from))).
		Set("status = ?", to).
		Set("updated_at = NOW()")

	if fields.WorkerID != nil {
		q = q.Set("worker_id = ?", *fields.WorkerID)
	} else if fields.ClearWorkerID {
		q = q.Set("worker_id = NULL")
	}
	if fields.LastHeartbeatAt != nil {
		q = q.Set("last_heartbeat_at = ?", *fields.LastHeartbeatAt)
	} else if fields.ClearHeartbeat {
		q = q.Set("last_heartbeat_at = NULL")
	}
	if fields.StartedAt != nil {
		q = q.Set("started_at = ?", *fields.StartedAt)
	} else if fields.ClearStartedAt {
		q = q.Set("started_at = NULL")
	}
	if fields.FinishedAt != nil {
		q = q.Set("finished_at = ?", *fields.FinishedAt)
	}
	if fields.ErrorMsg != nil {
		q = q.Set("error = ?", *fields.ErrorMsg)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *BunRepository) ClaimPendingRun(ctx context.Context, db bun.IDB, workerID string, now time.Time) (*PipelineRun, error) {
	// Step 1: SELECT FOR UPDATE SKIP LOCKED to pick one candidate id.
	var id uuid.UUID
	err := db.NewSelect().Model((*PipelineRun)(nil)).
		Column("id").
		Where("status = ?", RunPending).
		Order("created_at ASC").
		Limit(1).
		For("UPDATE SKIP LOCKED").
		Scan(ctx, &id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	// Step 2: status-guarded UPDATE — only succeeds if still pending.
	res, err := db.NewUpdate().Model((*PipelineRun)(nil)).
		Where("id = ?", id).
		Where("status = ?", RunPending).
		Set("status = ?", RunRunning).
		Set("worker_id = ?", workerID).
		Set("started_at = ?", now).
		Set("last_heartbeat_at = ?", now).
		Set("updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	return r.GetRun(ctx, db, id)
}

func (r *BunRepository) ScheduleAlreadyFired(
	ctx context.Context, db bun.IDB,
	pipelineName string, pipelineVersion int, firePoint time.Time,
) (bool, error) {
	n, err := db.NewSelect().Model((*PipelineRun)(nil)).
		Where("pipeline_name = ?", pipelineName).
		Where("pipeline_version = ?", pipelineVersion).
		Where("trigger_source = ?", TriggerSchedule).
		Where("created_at >= ?", firePoint).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *BunRepository) RefreshHeartbeat(ctx context.Context, db bun.IDB, runID uuid.UUID, workerID string, now time.Time) (int64, error) {
	res, err := db.NewUpdate().Model((*PipelineRun)(nil)).
		Where("id = ?", runID).
		Where("worker_id = ?", workerID).
		Where("status = ?", RunRunning).
		Set("last_heartbeat_at = ?", now).
		Set("updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *BunRepository) ReclaimStaleRun(
	ctx context.Context, db bun.IDB, runID uuid.UUID, threshold time.Duration,
) (*string, *time.Time, bool, error) {
	row := new(struct {
		WorkerID        *string    `bun:"worker_id"`
		LastHeartbeatAt *time.Time `bun:"last_heartbeat_at"`
	})
	cutoff := time.Now().UTC().Add(-threshold)
	err := db.NewSelect().Model((*PipelineRun)(nil)).
		Column("worker_id", "last_heartbeat_at").
		Where("id = ?", runID).
		Where("status = ?", RunRunning).
		Where("(last_heartbeat_at IS NULL OR last_heartbeat_at < ?)", cutoff).
		For("UPDATE").
		Scan(ctx, row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}
	_, err = db.NewUpdate().Model((*PipelineRun)(nil)).
		Where("id = ?", runID).
		Where("status = ?", RunRunning).
		Set("status = ?", RunPending).
		Set("worker_id = NULL").
		Set("last_heartbeat_at = NULL").
		Set("started_at = NULL").
		Set("updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	return row.WorkerID, row.LastHeartbeatAt, true, nil
}

func (r *BunRepository) UpsertWorkerSlot(ctx context.Context, db bun.IDB, slot *WorkerSlot) error {
	_, err := db.NewInsert().Model(slot).
		On("CONFLICT (worker_id) DO UPDATE").
		Set("last_heartbeat_at = EXCLUDED.last_heartbeat_at").
		Exec(ctx)
	return err
}

func (r *BunRepository) DeleteWorkerSlot(ctx context.Context, db bun.IDB, workerID string) error {
	_, err := db.NewDelete().Model((*WorkerSlot)(nil)).
		Where("worker_id = ?", workerID).
		Exec(ctx)
	return err
}

func (r *BunRepository) ListWorkers(ctx context.Context, db bun.IDB, staleThreshold time.Duration) ([]WorkerSummary, error) {
	cutoff := time.Now().UTC().Add(-staleThreshold)
	rows := []WorkerSummary{}
	// `active`  : per-worker_id COUNT + earliest start.
	// `current` : per-worker_id representative (run_id, pipeline_name)
	//             via DISTINCT ON. PostgreSQL-specific but fine —
	//             the project is Postgres-only.
	// Both LEFT-joined so idle slots survive with NULL aggregates.
	err := db.NewSelect().
		ColumnExpr("ws.worker_id AS worker_id").
		ColumnExpr("ws.hostname AS hostname").
		ColumnExpr("ws.pid AS pid").
		ColumnExpr("ws.slot_index AS slot_index").
		ColumnExpr("ws.started_at AS started_at").
		ColumnExpr("ws.last_heartbeat_at AS last_heartbeat_at").
		ColumnExpr("ws.tags AS tags").
		ColumnExpr("COALESCE(active.active_runs, 0)::int AS active_runs").
		ColumnExpr("active.earliest_run_start_at AS earliest_run_start_at").
		ColumnExpr("current.run_id::text AS current_run_id").
		ColumnExpr("current.pipeline_name AS current_pipeline").
		TableExpr("worker_slots AS ws").
		Join("LEFT JOIN (?) AS active ON active.worker_id = ws.worker_id",
			db.NewSelect().
				ColumnExpr("worker_id").
				ColumnExpr("COUNT(*) AS active_runs").
				ColumnExpr("MIN(started_at) AS earliest_run_start_at").
				TableExpr("pipeline_runs").
				Where("worker_id IS NOT NULL").
				Where("status IN (?)", bun.In([]string{string(RunRunning), string(RunCancelling)})).
				Group("worker_id"),
		).
		Join("LEFT JOIN (?) AS current ON current.worker_id = ws.worker_id",
			db.NewSelect().
				ColumnExpr("DISTINCT ON (worker_id) worker_id").
				ColumnExpr("id AS run_id").
				ColumnExpr("pipeline_name").
				TableExpr("pipeline_runs").
				Where("worker_id IS NOT NULL").
				Where("status IN (?)", bun.In([]string{string(RunRunning), string(RunCancelling)})).
				OrderExpr("worker_id, started_at ASC"),
		).
		Where("ws.last_heartbeat_at >= ?", cutoff).
		OrderExpr("ws.worker_id ASC").
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *BunRepository) ListRunsByWorker(ctx context.Context, db bun.IDB, workerID string, limit, offset int) ([]*PipelineRun, int, error) {
	q := db.NewSelect().Model((*PipelineRun)(nil)).
		Where("worker_id = ?", workerID)
	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	out := []*PipelineRun{}
	err = q.OrderExpr("started_at DESC NULLS LAST, id DESC").
		Limit(limit).Offset(offset).Scan(ctx, &out)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *BunRepository) ListStaleRunIDs(ctx context.Context, db bun.IDB, threshold time.Duration, limit int) ([]uuid.UUID, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	var ids []uuid.UUID
	err := db.NewSelect().Model((*PipelineRun)(nil)).
		Column("id").
		Where("status = ?", RunRunning).
		Where("(last_heartbeat_at IS NULL OR last_heartbeat_at < ?)", cutoff).
		OrderExpr("started_at ASC NULLS FIRST").
		Limit(limit).
		Scan(ctx, &ids)
	return ids, err
}

// -------------------------------------------------------------------
// StepRun
// -------------------------------------------------------------------

func (r *BunRepository) InsertStep(ctx context.Context, db bun.IDB, s *StepRun) error {
	_, err := db.NewInsert().Model(s).Returning("*").Exec(ctx)
	return err
}

func (r *BunRepository) GetStep(ctx context.Context, db bun.IDB, id uuid.UUID) (*StepRun, error) {
	out := new(StepRun)
	err := db.NewSelect().Model(out).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) LatestStepAttempt(ctx context.Context, db bun.IDB, runID uuid.UUID, stepName string, forUpdate bool) (*StepRun, error) {
	q := db.NewSelect().Model((*StepRun)(nil)).
		Where("pipeline_run_id = ?", runID).
		Where("step_name = ?", stepName).
		Order("attempt DESC").
		Limit(1)
	if forUpdate {
		q = q.For("UPDATE")
	}
	out := new(StepRun)
	err := q.Scan(ctx, out)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) ListStepsByRun(ctx context.Context, db bun.IDB, runID uuid.UUID) ([]*StepRun, error) {
	out := []*StepRun{}
	err := db.NewSelect().Model(&out).
		Where("pipeline_run_id = ?", runID).
		Order("created_at ASC", "attempt ASC").
		Scan(ctx)
	return out, err
}

func (r *BunRepository) UpdateStepStatusGuarded(
	ctx context.Context, db bun.IDB, id uuid.UUID,
	from []StepStatus, to StepStatus, fields StepUpdateFields,
) (int64, error) {
	q := db.NewUpdate().Model((*StepRun)(nil)).
		Where("id = ?", id).
		Where("status IN (?)", bun.In(toStrings(from))).
		Set("status = ?", to).
		Set("updated_at = NOW()")
	if fields.Result != nil {
		q = q.Set("result = ?", fields.Result)
	}
	if fields.ErrorMsg != nil {
		q = q.Set("error = ?", *fields.ErrorMsg)
	}
	if fields.FinishedAt != nil {
		q = q.Set("finished_at = ?", *fields.FinishedAt)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *BunRepository) UpdateRunCurrentStep(ctx context.Context, db bun.IDB, id uuid.UUID, stepName string) error {
	_, err := db.NewUpdate().Model((*PipelineRun)(nil)).
		Where("id = ?", id).
		Set("current_step = ?", stepName).
		Set("updated_at = NOW()").
		Exec(ctx)
	return err
}

func (r *BunRepository) AbortRunningStepsForRun(ctx context.Context, db bun.IDB, runID uuid.UUID) error {
	_, err := db.NewUpdate().Model((*StepRun)(nil)).
		Where("pipeline_run_id = ?", runID).
		Where("status IN (?)", bun.In([]string{string(StepRunning), string(StepAwaitingEvent)})).
		Set("status = ?", StepCancelled).
		Set("finished_at = NOW()").
		Set("updated_at = NOW()").
		Exec(ctx)
	return err
}

// -------------------------------------------------------------------
// EventWaiter
// -------------------------------------------------------------------

func (r *BunRepository) InsertWaiter(ctx context.Context, db bun.IDB, w *EventWaiter) error {
	_, err := db.NewInsert().Model(w).Returning("*").Exec(ctx)
	return err
}

func (r *BunRepository) GetWaiterByStepRun(ctx context.Context, db bun.IDB, stepRunID uuid.UUID, forUpdate bool) (*EventWaiter, error) {
	q := db.NewSelect().Model((*EventWaiter)(nil)).Where("step_run_id = ?", stepRunID)
	if forUpdate {
		q = q.For("UPDATE")
	}
	out := new(EventWaiter)
	err := q.Scan(ctx, out)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out, nil
}

func (r *BunRepository) DeleteWaiterByStepRun(ctx context.Context, db bun.IDB, stepRunID uuid.UUID) error {
	_, err := db.NewDelete().Model((*EventWaiter)(nil)).
		Where("step_run_id = ?", stepRunID).Exec(ctx)
	return err
}

// FindMatchingWaiterStepIDs uses Postgres JSONB containment match <@ payload.
func (r *BunRepository) FindMatchingWaiterStepIDs(ctx context.Context, db bun.IDB, eventType string, payload map[string]any) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := db.NewSelect().Model((*EventWaiter)(nil)).
		Column("step_run_id").
		Where("event_type = ?", eventType).
		Where("status = ?", WaiterWaiting).
		Where("match <@ ?::jsonb", payload).
		Scan(ctx, &ids)
	return ids, err
}

func (r *BunRepository) ListExpiredWaiterStepIDs(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := db.NewSelect().Model((*EventWaiter)(nil)).
		Column("step_run_id").
		Where("expires_at < ?", now).
		Order("expires_at ASC").
		Limit(limit).
		Scan(ctx, &ids)
	return ids, err
}

// -------------------------------------------------------------------
// helpers
// -------------------------------------------------------------------

func toStrings[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

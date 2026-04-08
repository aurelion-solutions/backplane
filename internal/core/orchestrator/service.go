// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ReclaimStaleThreshold is the cut-off used by ListStaleRunIDs /
// ReclaimStaleRun. Hard-coded for now; promotable to a runtime
// setting later.
const ReclaimStaleThreshold = 10 * time.Second

// Service is the sole writer to pipeline_runs, step_runs,
// pipeline_event_waiters.
//
// Every method takes a bun.IDB so the caller (route handler / runner /
// matcher) controls the transaction boundary. Service NEVER calls
// Commit or Rollback.
type Service struct {
	repo Repository
}

// NewService composes the service with a Repository.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// -------------------------------------------------------------------
// PipelineRun creation
// -------------------------------------------------------------------

// CreateRunInput is the payload-only view of POST /pipelines/{name}/runs.
type CreateRunInput struct {
	PipelineName    string
	PipelineVersion int
	Args            map[string]any
	TriggerSource   TriggerSource
	RetryOfRunID    *uuid.UUID
}

// CreateRunResult reports whether the row was newly inserted
// (Created=true) or whether the partial-UNIQUE dedupe returned an
// existing in-flight row (Created=false).
type CreateRunResult struct {
	Run     *PipelineRun
	Created bool
}

// CreateRun inserts a new pipeline_run row.
//
// When RetryOfRunID is nil and a duplicate in-flight row exists for
// (pipeline_name, pipeline_version, content_hash), returns the
// existing row with Created=false. Retries (RetryOfRunID != nil)
// bypass the UNIQUE and always insert a fresh row.
//
// Caller is responsible for the surrounding transaction.
func (s *Service) CreateRun(ctx context.Context, db bun.IDB, in CreateRunInput) (CreateRunResult, error) {
	if in.Args == nil {
		in.Args = map[string]any{}
	}
	hash := ContentHash(in.Args)

	insert := func() (*PipelineRun, error) {
		run := &PipelineRun{
			ID:              uuid.New(),
			PipelineName:    in.PipelineName,
			PipelineVersion: in.PipelineVersion,
			Args:            in.Args,
			ContentHash:     hash,
			Status:          RunPending,
			RetryOfRunID:    in.RetryOfRunID,
			TriggerSource:   in.TriggerSource,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		}
		if err := s.repo.InsertRun(ctx, db, run); err != nil {
			return nil, err
		}
		return run, nil
	}

	if in.RetryOfRunID != nil {
		run, err := insert()
		if err != nil {
			return CreateRunResult{}, err
		}
		return CreateRunResult{Run: run, Created: true}, nil
	}

	// Non-retry path: try INSERT; on partial-UNIQUE collision return
	// the in-flight row.
	run, err := insert()
	if err == nil {
		return CreateRunResult{Run: run, Created: true}, nil
	}
	if !isIdempotencyConflict(err) {
		return CreateRunResult{}, err
	}
	existing, lookupErr := s.repo.FindInflightRun(ctx, db, in.PipelineName, in.PipelineVersion, hash)
	if lookupErr != nil {
		return CreateRunResult{}, lookupErr
	}
	if existing == nil {
		// The conflicting row reached terminal status between INSERT
		// and SELECT — surface the raw insert error to the caller.
		return CreateRunResult{}, err
	}
	return CreateRunResult{Run: existing, Created: false}, nil
}

func isIdempotencyConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "uq_pipeline_runs_inflight_idempotency")
}

// CreateRetry inserts a fresh run that copies (pipeline_name,
// pipeline_version, args) from id and points retry_of_run_id at it.
// The source run must be in a terminal status (completed, failed,
// failed_timeout, cancelled).
func (s *Service) CreateRetry(ctx context.Context, db bun.IDB, id uuid.UUID) (*PipelineRun, error) {
	src, err := s.repo.GetRun(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if src.Status == RunCancelling {
		return nil, &ErrNotRetryable{RunID: id, Status: src.Status, Reason: "cancelling"}
	}
	if !src.Status.IsTerminal() {
		return nil, &ErrNotRetryable{RunID: id, Status: src.Status, Reason: "non_terminal"}
	}
	res, err := s.CreateRun(ctx, db, CreateRunInput{
		PipelineName:    src.PipelineName,
		PipelineVersion: src.PipelineVersion,
		Args:            src.Args,
		TriggerSource:   TriggerRetry,
		RetryOfRunID:    &id,
	})
	if err != nil {
		return nil, err
	}
	return res.Run, nil
}

// -------------------------------------------------------------------
// PipelineRun lookup
// -------------------------------------------------------------------

// GetRun returns one run by id.
func (s *Service) GetRun(ctx context.Context, db bun.IDB, id uuid.UUID) (*PipelineRun, error) {
	return s.repo.GetRun(ctx, db, id)
}

// ListRuns returns runs matching the filters with pagination.
func (s *Service) ListRuns(ctx context.Context, db bun.IDB, f ListRunsFilters) ([]*PipelineRun, int, error) {
	return s.repo.ListRuns(ctx, db, f)
}

// WorkerStaleThreshold drops registry rows whose heartbeat is older
// than this. Default = 3× heartbeat interval, so one missed tick is
// still visible but a long-gone process disappears.
const WorkerStaleThreshold = 15 * time.Second

// UpsertWorkerSlot writes/refreshes the registry row for a live slot.
func (s *Service) UpsertWorkerSlot(ctx context.Context, db bun.IDB, slot *WorkerSlot) error {
	return s.repo.UpsertWorkerSlot(ctx, db, slot)
}

// DeleteWorkerSlot removes the registry row on graceful shutdown.
func (s *Service) DeleteWorkerSlot(ctx context.Context, db bun.IDB, workerID string) error {
	return s.repo.DeleteWorkerSlot(ctx, db, workerID)
}

// ListWorkers reads the registry filtered by heartbeat freshness and
// joins per-worker active-run aggregates from pipeline_runs.
func (s *Service) ListWorkers(ctx context.Context, db bun.IDB) ([]WorkerSummary, error) {
	return s.repo.ListWorkers(ctx, db, WorkerStaleThreshold)
}

// ListRunsByWorker returns runs assigned to workerID with pagination.
func (s *Service) ListRunsByWorker(ctx context.Context, db bun.IDB, workerID string, limit, offset int) ([]*PipelineRun, int, error) {
	return s.repo.ListRunsByWorker(ctx, db, workerID, limit, offset)
}

// ListStepsByRun returns every step attempt for a run, oldest first.
func (s *Service) ListStepsByRun(ctx context.Context, db bun.IDB, runID uuid.UUID) ([]*StepRun, error) {
	return s.repo.ListStepsByRun(ctx, db, runID)
}

// LatestStepAttempt returns the most recent attempt for (run, step).
func (s *Service) LatestStepAttempt(ctx context.Context, db bun.IDB, runID uuid.UUID, stepName string) (*StepRun, error) {
	return s.repo.LatestStepAttempt(ctx, db, runID, stepName, false)
}

// ReadStatus returns the run status without locking — used by the
// runner's heartbeat refresher to detect 'cancelling' without
// coupling to ORM models.
func (s *Service) ReadStatus(ctx context.Context, db bun.IDB, id uuid.UUID) (RunStatus, error) {
	run, err := s.repo.GetRun(ctx, db, id)
	if err != nil {
		return "", err
	}
	return run.Status, nil
}

// -------------------------------------------------------------------
// PipelineRun lifecycle (status-guarded UPDATEs)
// -------------------------------------------------------------------

// ClaimPendingRun atomically claims one pending run for workerID.
// Returns nil + nil when the queue is empty or every row is locked by
// peers.
func (s *Service) ClaimPendingRun(ctx context.Context, db bun.IDB, workerID string) (*PipelineRun, error) {
	return s.repo.ClaimPendingRun(ctx, db, workerID, time.Now().UTC())
}

// RefreshHeartbeat ticks last_heartbeat_at on (worker_id, status=running).
// No event emitted — liveness signal, not a state transition.
func (s *Service) RefreshHeartbeat(ctx context.Context, db bun.IDB, runID uuid.UUID, workerID string) (bool, error) {
	n, err := s.repo.RefreshHeartbeat(ctx, db, runID, workerID, time.Now().UTC())
	return n > 0, err
}

// MarkRunAwaitingEvent transitions running → awaiting_event and
// clears worker_id / last_heartbeat_at so the runner slot is free.
func (s *Service) MarkRunAwaitingEvent(ctx context.Context, db bun.IDB, runID uuid.UUID) error {
	n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
		[]RunStatus{RunRunning}, RunAwaitingEvent,
		RunUpdateFields{ClearWorkerID: true, ClearHeartbeat: true})
	if err != nil {
		return err
	}
	if n == 0 {
		return s.conflictForRun(ctx, db, runID, []RunStatus{RunRunning})
	}
	return nil
}

// MarkRunRunningFromAwaiting transitions awaiting_event → running.
func (s *Service) MarkRunRunningFromAwaiting(ctx context.Context, db bun.IDB, runID uuid.UUID, workerID string) error {
	now := time.Now().UTC()
	n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
		[]RunStatus{RunAwaitingEvent}, RunRunning,
		RunUpdateFields{WorkerID: &workerID, LastHeartbeatAt: &now})
	if err != nil {
		return err
	}
	if n == 0 {
		return s.conflictForRun(ctx, db, runID, []RunStatus{RunAwaitingEvent})
	}
	return nil
}

// MarkRunCompleted transitions running|awaiting_event → completed.
//
// Cancel-vs-complete race: when the row is already in 'cancelling'
// the guarded UPDATE misses and we silently transition to 'cancelled'
// (the runner owns the pipeline.run.cancelled domain event).
func (s *Service) MarkRunCompleted(ctx context.Context, db bun.IDB, runID uuid.UUID) error {
	now := time.Now().UTC()
	n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
		[]RunStatus{RunRunning, RunAwaitingEvent}, RunCompleted,
		RunUpdateFields{FinishedAt: &now})
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	return s.handleCompleteOrFailMiss(ctx, db, runID, nil)
}

// MarkRunFailed transitions running|awaiting_event → failed.
func (s *Service) MarkRunFailed(ctx context.Context, db bun.IDB, runID uuid.UUID, errMsg string) error {
	now := time.Now().UTC()
	n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
		[]RunStatus{RunRunning, RunAwaitingEvent}, RunFailed,
		RunUpdateFields{FinishedAt: &now, ErrorMsg: &errMsg})
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	return s.handleCompleteOrFailMiss(ctx, db, runID, &errMsg)
}

func (s *Service) handleCompleteOrFailMiss(ctx context.Context, db bun.IDB, runID uuid.UUID, errMsg *string) error {
	run, err := s.repo.GetRun(ctx, db, runID)
	if err != nil {
		return err
	}
	if run.Status == RunCancelling {
		now := time.Now().UTC()
		n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
			[]RunStatus{RunCancelling}, RunCancelled,
			RunUpdateFields{FinishedAt: &now, ErrorMsg: errMsg})
		if err != nil {
			return err
		}
		if n == 0 {
			actual := run.Status
			return &ErrStateConflict{RunID: runID, Expected: []RunStatus{RunCancelling}, Actual: &actual}
		}
		return nil
	}
	actual := run.Status
	return &ErrStateConflict{
		RunID:    runID,
		Expected: []RunStatus{RunRunning, RunAwaitingEvent},
		Actual:   &actual,
	}
}

// MarkRunCancelled is the terminal transition from
// cancelling|pending → cancelled. Pending is allowed for the
// no-runner-yet branch where the run never acquired a worker.
func (s *Service) MarkRunCancelled(ctx context.Context, db bun.IDB, runID uuid.UUID) error {
	now := time.Now().UTC()
	n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
		[]RunStatus{RunCancelling, RunPending}, RunCancelled,
		RunUpdateFields{FinishedAt: &now})
	if err != nil {
		return err
	}
	if n == 0 {
		return s.conflictForRun(ctx, db, runID, []RunStatus{RunCancelling, RunPending})
	}
	return nil
}

// CancelOutcome reports the synchronous-vs-asynchronous flavour of
// RequestCancel: synchronous (sync=true → 'cancelled') for pending /
// awaiting_event runs, asynchronous (sync=false → 'cancelling') for
// running runs where the runner watcher owns the terminal transition.
type CancelOutcome struct {
	RunID  uuid.UUID `json:"run_id"`
	Status RunStatus `json:"status"`
	Sync   bool      `json:"sync"`
}

// RequestCancel dispatches cancel by current run status.
func (s *Service) RequestCancel(ctx context.Context, db bun.IDB, runID uuid.UUID) (CancelOutcome, error) {
	run, err := s.repo.GetRun(ctx, db, runID)
	if err != nil {
		return CancelOutcome{}, err
	}
	if run.Status == RunCancelling {
		return CancelOutcome{}, ErrAlreadyCancelling
	}
	if run.Status.IsTerminal() {
		return CancelOutcome{}, fmt.Errorf("%w: status=%s", ErrTerminal, run.Status)
	}
	switch run.Status {
	case RunPending:
		now := time.Now().UTC()
		n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
			[]RunStatus{RunPending}, RunCancelled,
			RunUpdateFields{FinishedAt: &now})
		if err != nil {
			return CancelOutcome{}, err
		}
		if n == 0 {
			return CancelOutcome{}, s.conflictForRun(ctx, db, runID, []RunStatus{RunPending})
		}
		_ = s.repo.AbortRunningStepsForRun(ctx, db, runID)
		return CancelOutcome{RunID: runID, Status: RunCancelled, Sync: true}, nil
	case RunAwaitingEvent:
		now := time.Now().UTC()
		n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
			[]RunStatus{RunAwaitingEvent}, RunCancelled,
			RunUpdateFields{FinishedAt: &now})
		if err != nil {
			return CancelOutcome{}, err
		}
		if n == 0 {
			return CancelOutcome{}, s.conflictForRun(ctx, db, runID, []RunStatus{RunAwaitingEvent})
		}
		if step, err := s.findLatestAwaiting(ctx, db, runID); err == nil && step != nil {
			_, _ = s.repo.UpdateStepStatusGuarded(ctx, db, step.ID,
				[]StepStatus{StepRunning, StepAwaitingEvent}, StepCancelled,
				StepUpdateFields{FinishedAt: &now})
			_ = s.repo.DeleteWaiterByStepRun(ctx, db, step.ID)
		}
		return CancelOutcome{RunID: runID, Status: RunCancelled, Sync: true}, nil
	default: // running
		n, err := s.repo.UpdateRunStatusGuarded(ctx, db, runID,
			[]RunStatus{RunRunning}, RunCancelling,
			RunUpdateFields{})
		if err != nil {
			return CancelOutcome{}, err
		}
		if n == 0 {
			return CancelOutcome{}, s.conflictForRun(ctx, db, runID, []RunStatus{RunRunning})
		}
		return CancelOutcome{RunID: runID, Status: RunCancelling, Sync: false}, nil
	}
}

func (s *Service) findLatestAwaiting(ctx context.Context, db bun.IDB, runID uuid.UUID) (*StepRun, error) {
	steps, err := s.repo.ListStepsByRun(ctx, db, runID)
	if err != nil {
		return nil, err
	}
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Status == StepAwaitingEvent {
			return steps[i], nil
		}
	}
	return nil, nil
}

// -------------------------------------------------------------------
// StepRun lifecycle
// -------------------------------------------------------------------

// CreateStepRun inserts a fresh attempt=1 step row and stamps
// pipeline_runs.current_step. Caller owns the transaction.
func (s *Service) CreateStepRun(ctx context.Context, db bun.IDB, runID uuid.UUID, stepName string, args map[string]any) (*StepRun, error) {
	now := time.Now().UTC()
	step := &StepRun{
		ID:            uuid.New(),
		PipelineRunID: runID,
		StepName:      stepName,
		Attempt:       1,
		Status:        StepRunning,
		Args:          coalesceArgs(args),
		StartedAt:     &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.InsertStep(ctx, db, step); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateRunCurrentStep(ctx, db, runID, stepName); err != nil {
		return nil, err
	}
	return step, nil
}

// MarkStepSucceeded transitions running → completed for a step.
func (s *Service) MarkStepSucceeded(ctx context.Context, db bun.IDB, stepID uuid.UUID, result map[string]any) error {
	now := time.Now().UTC()
	n, err := s.repo.UpdateStepStatusGuarded(ctx, db, stepID,
		[]StepStatus{StepRunning}, StepCompleted,
		StepUpdateFields{Result: result, FinishedAt: &now})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkStepFailed transitions running → failed for a step.
func (s *Service) MarkStepFailed(ctx context.Context, db bun.IDB, stepID uuid.UUID, errMsg string) error {
	now := time.Now().UTC()
	n, err := s.repo.UpdateStepStatusGuarded(ctx, db, stepID,
		[]StepStatus{StepRunning}, StepFailed,
		StepUpdateFields{ErrorMsg: &errMsg, FinishedAt: &now})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkStepAwaitingEvent transitions running → awaiting_event for a
// step. Paired with InsertEventWaiter and MarkRunAwaitingEvent.
func (s *Service) MarkStepAwaitingEvent(ctx context.Context, db bun.IDB, stepID uuid.UUID) error {
	n, err := s.repo.UpdateStepStatusGuarded(ctx, db, stepID,
		[]StepStatus{StepRunning}, StepAwaitingEvent,
		StepUpdateFields{})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// -------------------------------------------------------------------
// Event waiter lifecycle
// -------------------------------------------------------------------

// CreateEventWaiter inserts one waiter row. Caller-owned transaction;
// duplicate insert (one waiter per step attempt) raises through
// repository's UNIQUE constraint.
func (s *Service) CreateEventWaiter(ctx context.Context, db bun.IDB, stepID uuid.UUID, eventType string, match map[string]any, expiresAt time.Time) (*EventWaiter, error) {
	w := &EventWaiter{
		ID:        uuid.New(),
		StepRunID: stepID,
		EventType: eventType,
		Match:     coalesceArgs(match),
		ExpiresAt: expiresAt,
		Status:    WaiterWaiting,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.InsertWaiter(ctx, db, w); err != nil {
		return nil, err
	}
	return w, nil
}

// ResolveEventWaiter is the shared resolver used by both the matcher
// (Step 8) and the HITL endpoint (POST /pipelines/runs/{id}/steps/
// {step}/resolve).
//
// Returns true when this call won the race and the waiter was
// resolved; false when a concurrent resolver already won (no event,
// no state change).
func (s *Service) ResolveEventWaiter(ctx context.Context, db bun.IDB, stepRunID uuid.UUID, payload map[string]any) (bool, error) {
	waiter, err := s.repo.GetWaiterByStepRun(ctx, db, stepRunID, true)
	if err != nil {
		return false, err
	}
	_ = waiter
	step, err := s.repo.GetStep(ctx, db, stepRunID)
	if err != nil {
		return false, err
	}
	if err := s.repo.DeleteWaiterByStepRun(ctx, db, stepRunID); err != nil {
		return false, err
	}
	now := time.Now().UTC()
	n, err := s.repo.UpdateStepStatusGuarded(ctx, db, stepRunID,
		[]StepStatus{StepAwaitingEvent}, StepCompleted,
		StepUpdateFields{Result: coalesceArgs(payload), FinishedAt: &now})
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	// Re-activate run (awaiting_event → pending) so the runner picks
	// it up again. Guarded by status so a cancelled run is unaffected.
	_, err = s.repo.UpdateRunStatusGuarded(ctx, db, step.PipelineRunID,
		[]RunStatus{RunAwaitingEvent}, RunPending, RunUpdateFields{})
	return true, err
}

// FindMatchingWaiterStepIDs is the matcher's read path. No locking.
func (s *Service) FindMatchingWaiterStepIDs(ctx context.Context, db bun.IDB, eventType string, payload map[string]any) ([]uuid.UUID, error) {
	return s.repo.FindMatchingWaiterStepIDs(ctx, db, eventType, payload)
}

// ListExpiredWaiterStepIDs is the beat sweep's peek query.
func (s *Service) ListExpiredWaiterStepIDs(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]uuid.UUID, error) {
	return s.repo.ListExpiredWaiterStepIDs(ctx, db, now, limit)
}

// ExpireEventWaiter is the beat-side counterpart to ResolveEventWaiter.
//
// Drops the waiter row, transitions the step + run from awaiting_event
// → failed_timeout, stamps the error message "event_timeout". Returns
// (true, runID) on success and (false, nil) when the waiter is
// already gone (raced by matcher / cancel) or the step / run is no
// longer awaiting_event.
func (s *Service) ExpireEventWaiter(ctx context.Context, db bun.IDB, stepRunID uuid.UUID) (bool, *uuid.UUID, error) {
	waiter, err := s.repo.GetWaiterByStepRun(ctx, db, stepRunID, true)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}
	_ = waiter
	step, err := s.repo.GetStep(ctx, db, stepRunID)
	if err != nil {
		// orphan waiter — delete and bail.
		_ = s.repo.DeleteWaiterByStepRun(ctx, db, stepRunID)
		return false, nil, nil
	}
	if step.Status != StepAwaitingEvent {
		_ = s.repo.DeleteWaiterByStepRun(ctx, db, stepRunID)
		return false, nil, nil
	}
	run, err := s.repo.GetRun(ctx, db, step.PipelineRunID)
	if err != nil {
		_ = s.repo.DeleteWaiterByStepRun(ctx, db, stepRunID)
		return false, nil, nil
	}
	if run.Status != RunAwaitingEvent {
		_ = s.repo.DeleteWaiterByStepRun(ctx, db, stepRunID)
		return false, nil, nil
	}
	if err := s.repo.DeleteWaiterByStepRun(ctx, db, stepRunID); err != nil {
		return false, nil, err
	}
	now := time.Now().UTC()
	timeoutMsg := "event_timeout"
	n, err := s.repo.UpdateStepStatusGuarded(ctx, db, stepRunID,
		[]StepStatus{StepAwaitingEvent}, StepFailedTimeout,
		StepUpdateFields{ErrorMsg: &timeoutMsg, FinishedAt: &now})
	if err != nil {
		return false, nil, err
	}
	if n == 0 {
		return false, nil, nil
	}
	n, err = s.repo.UpdateRunStatusGuarded(ctx, db, run.ID,
		[]RunStatus{RunAwaitingEvent}, RunFailedTimeout,
		RunUpdateFields{FinishedAt: &now, ErrorMsg: &timeoutMsg})
	if err != nil {
		return false, nil, err
	}
	if n == 0 {
		return false, nil, nil
	}
	rid := run.ID
	return true, &rid, nil
}

// IsScheduleAlreadyFired returns true when a schedule-triggered run
// for (pipelineName, pipelineVersion) already exists with
// created_at >= firePoint. Used by Beat to dedupe within the cron
// window.
func (s *Service) IsScheduleAlreadyFired(
	ctx context.Context, db bun.IDB,
	pipelineName string, pipelineVersion int, firePoint time.Time,
) (bool, error) {
	return s.repo.ScheduleAlreadyFired(ctx, db, pipelineName, pipelineVersion, firePoint)
}

// -------------------------------------------------------------------
// Reclaim
// -------------------------------------------------------------------

// ReclaimResult reports what ReclaimStaleRun did so the caller can
// shape the heartbeat_lost event.
type ReclaimResult struct {
	OK                bool
	PreviousWorkerID  *string
	StaleForSeconds   float64
	AbortedStepRunID  *uuid.UUID
	AbortedStepName   string
	AbortedAttempt    int
}

// ListStaleRunIDs returns IDs of running rows whose
// last_heartbeat_at is older than ReclaimStaleThreshold (or NULL).
func (s *Service) ListStaleRunIDs(ctx context.Context, db bun.IDB, limit int) ([]uuid.UUID, error) {
	return s.repo.ListStaleRunIDs(ctx, db, ReclaimStaleThreshold, limit)
}

// ReclaimStaleRun atomically releases one stale run (status=running,
// stale heartbeat). Returns OK=false when the row is already
// reclaimed / fresh / wrong status.
func (s *Service) ReclaimStaleRun(ctx context.Context, db bun.IDB, runID uuid.UUID) (ReclaimResult, error) {
	worker, lastHb, ok, err := s.repo.ReclaimStaleRun(ctx, db, runID, ReclaimStaleThreshold)
	if err != nil {
		return ReclaimResult{}, err
	}
	if !ok {
		return ReclaimResult{OK: false}, nil
	}
	out := ReclaimResult{OK: true, PreviousWorkerID: worker}
	if lastHb != nil {
		out.StaleForSeconds = time.Since(*lastHb).Seconds()
	}
	// Abort the latest running step attempt (if any).
	step, err := s.repo.LatestStepAttempt(ctx, db, runID, "", true)
	if err == nil && step != nil && step.Status == StepRunning {
		now := time.Now().UTC()
		errMsg := "reclaimed: heartbeat lost"
		_, _ = s.repo.UpdateStepStatusGuarded(ctx, db, step.ID,
			[]StepStatus{StepRunning}, StepAborted,
			StepUpdateFields{ErrorMsg: &errMsg, FinishedAt: &now})
		out.AbortedStepRunID = &step.ID
		out.AbortedStepName = step.StepName
		out.AbortedAttempt = step.Attempt
	}
	return out, nil
}

// -------------------------------------------------------------------
// helpers
// -------------------------------------------------------------------

func (s *Service) conflictForRun(ctx context.Context, db bun.IDB, runID uuid.UUID, expected []RunStatus) error {
	run, err := s.repo.GetRun(ctx, db, runID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &ErrStateConflict{RunID: runID, Expected: expected, Actual: nil}
		}
		return err
	}
	actual := run.Status
	return &ErrStateConflict{RunID: runID, Expected: expected, Actual: &actual}
}

func coalesceArgs(args map[string]any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	return args
}

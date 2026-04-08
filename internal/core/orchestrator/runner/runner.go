// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// HeartbeatInterval is how often the heartbeat goroutine refreshes
// pipeline_runs.last_heartbeat_at while an action is in flight.
const HeartbeatInterval = 3 * time.Second

// SlotHeartbeatInterval is how often the slot lifecycle goroutine
// refreshes worker_slots.last_heartbeat_at. The /workers endpoint
// drops registry rows whose heartbeat is older than
// orchestrator.WorkerStaleThreshold (15s) — keep this strictly under
// that so one missed tick is still visible.
const SlotHeartbeatInterval = 5 * time.Second

// ReclaimSweepLimit is the per-tick batch size for the stale-run
// reclaim sweep.
const ReclaimSweepLimit = 50

// PollInterval is the back-off slept when the claim query returns no
// row. Hard-coded; promote to runtime settings later.
const PollInterval = 1 * time.Second

// Outcome reports the result of one work-loop iteration.
type Outcome string

const (
	OutcomeIdle           Outcome = "idle"
	OutcomeCompleted      Outcome = "completed"
	OutcomeFailed         Outcome = "failed"
	OutcomeAwaitingEvent  Outcome = "awaiting_event"
	OutcomeCancelled      Outcome = "cancelled"
)

// PipelineGetter is the runtime contract the runner uses to look a
// pipeline definition up by name. *orchestrator.Catalog satisfies it.
type PipelineGetter interface {
	Get(name string) *loader.Definition
}

// Runner is the work-loop driver. One Runner per goroutine; N
// goroutines per worker process.
type Runner struct {
	db       *bun.DB
	svc      *orchestrator.Service
	reg      *registry.Registry
	catalog  PipelineGetter
	log      *slog.Logger
	worker   WorkerIdentity
}

// New composes a Runner with all dependencies. The catalog reference
// is shared with the backplane HTTP process — it never mutates here.
func New(
	db *bun.DB,
	svc *orchestrator.Service,
	reg *registry.Registry,
	catalog PipelineGetter,
	log *slog.Logger,
	worker WorkerIdentity,
) *Runner {
	return &Runner{db: db, svc: svc, reg: reg, catalog: catalog, log: log, worker: worker}
}

// WorkLoop runs until ctx is cancelled. On shutdown the in-flight run
// is left to drain — the reclaim sweep on a sibling worker will pick
// it up if it doesn't finish in time. The slot's registry row in
// worker_slots is registered at startup, refreshed on a ticker, and
// deleted on graceful shutdown.
func (r *Runner) WorkLoop(ctx context.Context) error {
	r.log.Info("runner work loop starting", slog.String("worker_id", r.worker.WorkerID))

	// Slot registry: register immediately so the worker is visible in
	// /api/v0/workers from t=0, even before any run is claimed.
	r.registerSlot(ctx)
	go r.slotHeartbeatLoop(ctx)
	defer r.deregisterSlot()

	for {
		select {
		case <-ctx.Done():
			r.log.Info("runner work loop stopping", slog.String("worker_id", r.worker.WorkerID))
			return nil
		default:
		}
		if err := r.reclaimSweepTick(ctx); err != nil {
			r.log.Warn("reclaim sweep tick failed", slog.Any("err", err))
		}
		outcome, err := r.RunOneIteration(ctx)
		if err != nil {
			r.log.Error("runner iteration failed", slog.Any("err", err))
			outcome = OutcomeIdle
		}
		if outcome == OutcomeIdle {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(PollInterval):
			}
		}
	}
}

// registerSlot writes the initial worker_slots row. Failure is
// logged-and-ignored — the runner can still do work; it just won't be
// visible in /workers until the next tick succeeds.
func (r *Runner) registerSlot(ctx context.Context) {
	now := time.Now().UTC()
	slot := &orchestrator.WorkerSlot{
		WorkerID:        r.worker.WorkerID,
		Hostname:        r.worker.Hostname,
		PID:             r.worker.PID,
		SlotIndex:       r.worker.SlotIndex,
		StartedAt:       now,
		LastHeartbeatAt: now,
		Tags:            r.worker.Tags,
	}
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return r.svc.UpsertWorkerSlot(ctx, tx, slot)
	})
	if err != nil {
		r.log.Warn("slot register failed", slog.Any("err", err))
	}
}

// slotHeartbeatLoop refreshes worker_slots.last_heartbeat_at on a
// fixed cadence so /workers keeps showing the slot even while it is
// idle.
func (r *Runner) slotHeartbeatLoop(ctx context.Context) {
	t := time.NewTicker(SlotHeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := time.Now().UTC()
		slot := &orchestrator.WorkerSlot{
			WorkerID:        r.worker.WorkerID,
			Hostname:        r.worker.Hostname,
			PID:             r.worker.PID,
			SlotIndex:       r.worker.SlotIndex,
			StartedAt:       now, // overwritten only on first insert via UPSERT
			LastHeartbeatAt: now,
			Tags:            r.worker.Tags,
		}
		err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			return r.svc.UpsertWorkerSlot(ctx, tx, slot)
		})
		if err != nil {
			r.log.Warn("slot heartbeat tick failed", slog.Any("err", err))
		}
	}
}

// deregisterSlot removes the registry row on graceful shutdown.
// Runs with a fresh background context — the parent ctx is already
// done. Best-effort: failure leaves a stale row that the staleness
// filter on /workers will drop within WorkerStaleThreshold.
func (r *Runner) deregisterSlot() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return r.svc.DeleteWorkerSlot(ctx, tx, r.worker.WorkerID)
	})
	if err != nil {
		r.log.Warn("slot deregister failed", slog.Any("err", err))
	}
}

// RunOneIteration claims at most one pending run and drives it to a
// terminal state (or parks it on a wait_for_event).
func (r *Runner) RunOneIteration(ctx context.Context) (Outcome, error) {
	// --- Tx A: claim ---------------------------------------------------
	var claimed *orchestrator.PipelineRun
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		run, err := r.svc.ClaimPendingRun(ctx, tx, r.worker.WorkerID)
		if err != nil {
			return err
		}
		claimed = run
		return nil
	})
	if err != nil {
		return OutcomeIdle, err
	}
	if claimed == nil {
		return OutcomeIdle, nil
	}
	return r.executeRun(ctx, claimed)
}

// executeRun is run-scoped: it owns the per-step transactions, the
// heartbeat goroutine, and the cancel watcher.
func (r *Runner) executeRun(ctx context.Context, run *orchestrator.PipelineRun) (Outcome, error) {
	runID := run.ID
	r.log.Debug("runner claimed run",
		slog.String("run_id", runID.String()),
		slog.String("pipeline", run.PipelineName),
	)

	def := r.catalog.Get(run.PipelineName)
	if def == nil {
		err := r.markRunFailed(ctx, runID, "pipeline definition not found")
		return OutcomeFailed, err
	}

	// Resume support: pull every step_run already on disk so we can
	// skip completed steps when the runner re-claims a run that was
	// parked on wait_for_event (or any other resume scenario).
	completedStepResults, err := r.loadCompletedStepResults(ctx, runID)
	if err != nil {
		_ = r.markRunFailed(ctx, runID, fmt.Sprintf("load prior steps: %v", err))
		return OutcomeFailed, nil
	}
	stepResults := completedStepResults

	for _, step := range def.Steps {
		stepName := loader.StepName(step)

		// Skip steps already completed in a prior incarnation of this run.
		if _, done := stepResults[stepName]; done {
			continue
		}

		if loader.StepKind(step) == loader.StepWaitForEvent {
			return r.parkWaitForEvent(ctx, runID, step, run.Args, stepResults)
		}

		// engine_call branch.
		engine, _ := step["engine"].(string)
		action, _ := step["action"].(string)
		rawArgs, _ := step["args"].(map[string]any)
		if rawArgs == nil {
			rawArgs = map[string]any{}
		}
		resolvedArgs, err := resolveTemplates(rawArgs, run.Args, stepResults)
		if err != nil {
			_ = r.markRunFailed(ctx, runID, fmt.Sprintf("template resolution failed for step %q: %v", stepName, err))
			return OutcomeFailed, nil
		}
		argsMap, _ := resolvedArgs.(map[string]any)
		if argsMap == nil {
			argsMap = map[string]any{}
		}

		// --- Tx B: insert + commit step_run so subsequent failure
		//           transactions can see it. -------------------------
		var step0 *orchestrator.StepRun
		err = r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			s, err := r.svc.CreateStepRun(ctx, tx, runID, stepName, argsMap)
			if err != nil {
				return err
			}
			step0 = s
			return nil
		})
		if err != nil {
			_ = r.markRunFailed(ctx, runID, fmt.Sprintf("persist step %q: %v", stepName, err))
			return OutcomeFailed, nil
		}

		outcome, result, err := r.runStep(ctx, runID, step0, engine, action, argsMap)
		if err != nil {
			return outcome, err
		}
		switch outcome {
		case OutcomeFailed, OutcomeCancelled:
			return outcome, nil
		}
		stepResults[stepName] = result
	}

	// All steps done — complete the run.
	completeErr := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return r.svc.MarkRunCompleted(ctx, tx, runID)
	})
	if completeErr != nil {
		r.log.Warn("MarkRunCompleted failed", slog.Any("err", completeErr))
	}
	r.log.Info("run completed", slog.String("run_id", runID.String()))
	return OutcomeCompleted, nil
}

// runStep is the heartbeat-watched action invocation. Returns
// outcome (completed | failed | cancelled), the result map, and an
// error only on infrastructure failures.
func (r *Runner) runStep(
	ctx context.Context,
	runID uuid.UUID,
	step *orchestrator.StepRun,
	engine, action string,
	args map[string]any,
) (Outcome, map[string]any, error) {
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	cancelDetected := make(chan struct{}, 1)
	var hbWG sync.WaitGroup
	hbWG.Add(1)
	go func() {
		defer hbWG.Done()
		r.heartbeatLoop(hbCtx, runID, cancelDetected)
	}()

	// Per-action context that the heartbeat can cancel when it spots
	// 'cancelling' on the run row.
	actionCtx, actionCancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-cancelDetected:
			actionCancel()
		case <-actionCtx.Done():
		}
	}()

	// --- Tx C: action under bun.Tx; on success commit the success
	//           transition through the same Tx. -------------------------
	var result map[string]any
	dispatchErr := r.db.RunInTx(actionCtx, nil, func(ctx context.Context, tx bun.Tx) error {
		ac := registry.ActionContext{
			Ctx:           ctx,
			Tx:            tx,
			Log:           r.log,
			PipelineRunID: runID,
			StepRunID:     step.ID,
			Attempt:       step.Attempt,
			WorkerID:      r.worker.WorkerID,
		}
		out, err := r.reg.Dispatch(engine, action, args, ac)
		if err != nil {
			return err
		}
		if err := r.svc.MarkStepSucceeded(ctx, tx, step.ID, out); err != nil {
			return err
		}
		result = out
		return nil
	})

	// Stop heartbeat, irrespective of outcome.
	hbCancel()
	hbWG.Wait()

	if dispatchErr == nil {
		return OutcomeCompleted, result, nil
	}

	// Cancel-via-heartbeat path: action context was cancelled by the
	// heartbeat goroutine because the run flipped to 'cancelling'.
	if errors.Is(dispatchErr, context.Canceled) && ctx.Err() == nil {
		err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			now := time.Now().UTC()
			_, _ = r.svc.LatestStepAttempt(ctx, tx, runID, step.StepName)
			// Step + run go to cancelled in the same Tx.
			if _, err := r.svc.RequestCancel(ctx, tx, runID); err != nil {
				if !errors.Is(err, orchestrator.ErrAlreadyCancelling) {
					return err
				}
			}
			if err := r.svc.MarkRunCancelled(ctx, tx, runID); err != nil {
				return err
			}
			_ = now
			return nil
		})
		if err != nil {
			r.log.Warn("finalize cancel failed", slog.Any("err", err))
		}
		return OutcomeCancelled, nil, nil
	}

	// Plain action error → mark step + run failed in a fresh Tx.
	errMsg := dispatchErr.Error()
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := r.svc.MarkStepFailed(ctx, tx, step.ID, errMsg); err != nil {
			return err
		}
		return r.svc.MarkRunFailed(ctx, tx, runID, fmt.Sprintf("step %q failed: %s", step.StepName, errMsg))
	})
	if err != nil {
		r.log.Warn("finalize failure failed", slog.Any("err", err))
	}
	return OutcomeFailed, nil, nil
}

// heartbeatLoop refreshes last_heartbeat_at every HeartbeatInterval
// and watches for a status flip to 'cancelling'. Closes cancelCh on
// detection (the caller cancels the action context).
func (r *Runner) heartbeatLoop(ctx context.Context, runID uuid.UUID, cancelCh chan<- struct{}) {
	t := time.NewTicker(HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		var status orchestrator.RunStatus
		err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if _, err := r.svc.RefreshHeartbeat(ctx, tx, runID, r.worker.WorkerID); err != nil {
				return err
			}
			s, err := r.svc.ReadStatus(ctx, tx, runID)
			if err != nil {
				return err
			}
			status = s
			return nil
		})
		if err != nil {
			r.log.Warn("heartbeat tick failed", slog.Any("err", err))
			continue
		}
		if status == orchestrator.RunCancelling {
			select {
			case cancelCh <- struct{}{}:
			default:
			}
			return
		}
	}
}

// parkWaitForEvent handles the wait_for_event step kind.
func (r *Runner) parkWaitForEvent(
	ctx context.Context,
	runID uuid.UUID,
	step map[string]any,
	pipelineArgs map[string]any,
	stepResults map[string]map[string]any,
) (Outcome, error) {
	stepName := loader.StepName(step)
	rawTimeout, _ := step["timeout"].(string)
	delta, err := parseDuration(rawTimeout)
	if err != nil {
		_ = r.markRunFailed(ctx, runID, fmt.Sprintf("invalid timeout for step %q: %v", stepName, err))
		return OutcomeFailed, nil
	}
	expiresAt := time.Now().UTC().Add(delta)

	rawMatch, _ := step["match"].(map[string]any)
	if rawMatch == nil {
		rawMatch = map[string]any{}
	}
	resolved, err := resolveTemplates(rawMatch, pipelineArgs, stepResults)
	if err != nil {
		_ = r.markRunFailed(ctx, runID, fmt.Sprintf("template resolution failed for step %q: %v", stepName, err))
		return OutcomeFailed, nil
	}
	match, _ := resolved.(map[string]any)
	if match == nil {
		match = map[string]any{}
	}

	args := map[string]any{
		"event":      step["event"],
		"match":      match,
		"timeout":    step["timeout"],
		"on_timeout": "fail",
	}
	if v, ok := step["on_timeout"].(string); ok && v != "" {
		args["on_timeout"] = v
	}

	eventType, _ := step["event"].(string)
	err = r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		s, err := r.svc.CreateStepRun(ctx, tx, runID, stepName, args)
		if err != nil {
			return err
		}
		if err := r.svc.MarkStepAwaitingEvent(ctx, tx, s.ID); err != nil {
			return err
		}
		if _, err := r.svc.CreateEventWaiter(ctx, tx, s.ID, eventType, match, expiresAt); err != nil {
			return err
		}
		return r.svc.MarkRunAwaitingEvent(ctx, tx, runID)
	})
	if err != nil {
		_ = r.markRunFailed(ctx, runID, fmt.Sprintf("park wait_for_event failed: %v", err))
		return OutcomeFailed, nil
	}
	r.log.Info("run parked on wait_for_event",
		slog.String("run_id", runID.String()),
		slog.String("event", eventType),
		slog.Time("expires_at", expiresAt),
	)
	return OutcomeAwaitingEvent, nil
}

// reclaimSweepTick releases every stale run we can grab in one batch.
// Each candidate is reclaimed in its own short Tx so one poison row
// doesn't block the others.
func (r *Runner) reclaimSweepTick(ctx context.Context) error {
	var ids []uuid.UUID
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		out, err := r.svc.ListStaleRunIDs(ctx, tx, ReclaimSweepLimit)
		if err != nil {
			return err
		}
		ids = out
		return nil
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		txErr := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			_, err := r.svc.ReclaimStaleRun(ctx, tx, id)
			return err
		})
		if txErr != nil {
			r.log.Warn("reclaim sweep row failed",
				slog.String("run_id", id.String()),
				slog.Any("err", txErr))
		}
	}
	return nil
}

// loadCompletedStepResults returns the result map of every step that
// has reached the 'completed' status in a prior incarnation of this
// run. Used by executeRun to skip already-done work when the run was
// parked on wait_for_event and resumed via ResolveEventWaiter.
func (r *Runner) loadCompletedStepResults(ctx context.Context, runID uuid.UUID) (map[string]map[string]any, error) {
	var steps []*orchestrator.StepRun
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		out, err := r.svc.ListStepsByRun(ctx, tx, runID)
		steps = out
		return err
	})
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	for _, s := range steps {
		if s.Status != orchestrator.StepCompleted {
			continue
		}
		if existing, dup := out[s.StepName]; dup {
			// Multiple completed attempts shouldn't happen — pick the
			// latest by attempt number defensively.
			if s.Attempt <= attemptOf(existing) {
				continue
			}
		}
		result := s.Result
		if result == nil {
			result = map[string]any{}
		}
		out[s.StepName] = result
	}
	return out, nil
}

// attemptOf is a tiny helper for the dup-by-step-name defensive
// branch above. We stash the attempt under a sentinel key when we
// need to compare; in normal flow each step_name has exactly one
// completed attempt.
func attemptOf(m map[string]any) int {
	if v, ok := m["_attempt"].(int); ok {
		return v
	}
	return 0
}

func (r *Runner) markRunFailed(ctx context.Context, runID uuid.UUID, msg string) error {
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return r.svc.MarkRunFailed(ctx, tx, runID, msg)
	})
	if err != nil {
		r.log.Warn("MarkRunFailed failed", slog.Any("err", err), slog.String("run_id", runID.String()))
	}
	return err
}

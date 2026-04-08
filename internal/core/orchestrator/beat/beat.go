// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package beat

import (
	"context"
	"log/slog"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// TickInterval is how often the loop wakes up to fire / sweep.
// Hard-coded to mirror the kernel's 10 s cadence.
const TickInterval = 10 * time.Second

// SweepBatchLimit caps the number of expired waiters processed in one
// tick so a stall doesn't snowball.
const SweepBatchLimit = 100

// AdvisoryLockKey is the 64-bit integer "AURELBEA7" used as the
// per-tick pg_try_advisory_lock key. Lives in shared code so a future
// observability tool can decode it.
const AdvisoryLockKey int64 = 0x4155_5245_4C42_4541

// TickResult summarises what one tick did.
type TickResult struct {
	LockAcquired      bool
	FiredRunIDs       []uuid.UUID
	SkippedSchedules  int
	ExpiredRunIDs     []uuid.UUID
	ExpireFailures    int
}

// Catalog is the runtime contract Beat uses to enumerate pipeline
// definitions on every tick.
type Catalog interface {
	All() []*loader.Definition
}

// Beat is the periodic scheduler. One instance per process; advisory
// lock ensures only one replica fires at a time.
type Beat struct {
	db      *bun.DB
	svc     *orchestrator.Service
	catalog Catalog
	log     *slog.Logger
}

// New composes a Beat.
func New(db *bun.DB, svc *orchestrator.Service, catalog Catalog, log *slog.Logger) *Beat {
	return &Beat{db: db, svc: svc, catalog: catalog, log: log}
}

// Loop runs Tick every TickInterval until ctx is cancelled.
func (b *Beat) Loop(ctx context.Context) error {
	b.log.Info("beat loop starting")
	t := time.NewTicker(TickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			b.log.Info("beat loop stopping")
			return nil
		case <-t.C:
		}
		if _, err := b.Tick(ctx, time.Now().UTC()); err != nil {
			b.log.Warn("beat tick failed", slog.Any("err", err))
		}
	}
}

// Tick performs one beat cycle: acquire advisory lock, fire due
// schedules, sweep expired waiters, release lock.
func (b *Beat) Tick(ctx context.Context, now time.Time) (TickResult, error) {
	result := TickResult{}
	err := b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Acquire the per-tick advisory lock; bail out cleanly on contention.
		var acquired bool
		row := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(?)", AdvisoryLockKey)
		if err := row.Scan(&acquired); err != nil {
			return err
		}
		result.LockAcquired = acquired
		if !acquired {
			return nil
		}
		defer func() {
			_, _ = tx.ExecContext(ctx, "SELECT pg_advisory_unlock(?)", AdvisoryLockKey)
		}()

		b.fireSchedules(ctx, tx, now, &result)
		b.sweepExpiredWaiters(ctx, tx, now, &result)
		return nil
	})
	return result, err
}

func (b *Beat) fireSchedules(ctx context.Context, tx bun.Tx, now time.Time, result *TickResult) {
	for _, defn := range b.catalog.All() {
		for _, trigger := range defn.Triggers {
			if t, _ := trigger["type"].(string); t != "schedule" {
				continue
			}
			cronExpr, _ := trigger["cron"].(string)
			everyExpr, _ := trigger["every"].(string)
			firePoint, err := PreviousFirePoint(now, cronExpr, everyExpr)
			if err != nil {
				b.log.Warn("beat schedule parse failed",
					slog.String("pipeline", defn.Name), slog.Any("err", err))
				continue
			}
			fired, err := b.svc.IsScheduleAlreadyFired(ctx, tx, defn.Name, defn.Version, firePoint)
			if err != nil {
				b.log.Warn("beat schedule lookup failed",
					slog.String("pipeline", defn.Name), slog.Any("err", err))
				continue
			}
			if fired {
				result.SkippedSchedules++
				continue
			}
			args := map[string]any{}
			if a, ok := trigger["args"].(map[string]any); ok {
				for k, v := range a {
					args[k] = v
				}
			}
			args["_scheduled_at"] = now.Format(time.RFC3339Nano)
			res, err := b.svc.CreateRun(ctx, tx, orchestrator.CreateRunInput{
				PipelineName:    defn.Name,
				PipelineVersion: defn.Version,
				Args:            args,
				TriggerSource:   orchestrator.TriggerSchedule,
			})
			if err != nil {
				b.log.Warn("beat fire failed",
					slog.String("pipeline", defn.Name), slog.Any("err", err))
				continue
			}
			if res.Created {
				result.FiredRunIDs = append(result.FiredRunIDs, res.Run.ID)
				b.log.Info("beat schedule fired",
					slog.String("pipeline", defn.Name),
					slog.String("run_id", res.Run.ID.String()),
					slog.Time("fire_point", firePoint))
			} else {
				result.SkippedSchedules++
			}
		}
	}
}

func (b *Beat) sweepExpiredWaiters(ctx context.Context, tx bun.Tx, now time.Time, result *TickResult) {
	ids, err := b.svc.ListExpiredWaiterStepIDs(ctx, tx, now, SweepBatchLimit)
	if err != nil {
		b.log.Warn("beat waiter scan failed", slog.Any("err", err))
		return
	}
	for _, stepID := range ids {
		ok, runID, err := b.svc.ExpireEventWaiter(ctx, tx, stepID)
		if err != nil {
			result.ExpireFailures++
			b.log.Warn("beat waiter expire failed",
				slog.String("step_run_id", stepID.String()), slog.Any("err", err))
			continue
		}
		if ok && runID != nil {
			result.ExpiredRunIDs = append(result.ExpiredRunIDs, *runID)
			b.log.Info("beat waiter expired",
				slog.String("step_run_id", stepID.String()),
				slog.String("run_id", runID.String()))
		}
	}
}

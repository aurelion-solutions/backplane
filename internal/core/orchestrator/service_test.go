// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// memRepo is an in-memory Repository for unit tests. It does NOT
// model the partial-UNIQUE index — tests that need that race must
// drive memRepo through the seed* helpers below.
type memRepo struct {
	mu      sync.Mutex
	runs    map[uuid.UUID]*PipelineRun
	steps   map[uuid.UUID]*StepRun
	waiters map[uuid.UUID]*EventWaiter // keyed by step_run_id
	slots   map[string]*WorkerSlot     // keyed by worker_id
}

func newMemRepo() *memRepo {
	return &memRepo{
		runs:    map[uuid.UUID]*PipelineRun{},
		steps:   map[uuid.UUID]*StepRun{},
		waiters: map[uuid.UUID]*EventWaiter{},
		slots:   map[string]*WorkerSlot{},
	}
}

// --- PipelineRun ----------------------------------------------------

func (r *memRepo) InsertRun(_ context.Context, _ bun.IDB, run *PipelineRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Mimic partial-UNIQUE.
	if run.RetryOfRunID == nil {
		for _, existing := range r.runs {
			if existing.PipelineName == run.PipelineName &&
				existing.PipelineVersion == run.PipelineVersion &&
				existing.ContentHash == run.ContentHash &&
				existing.RetryOfRunID == nil &&
				!existing.Status.IsTerminal() &&
				existing.Status != "" {
				return fmt.Errorf("uq_pipeline_runs_inflight_idempotency conflict")
			}
		}
	}
	r.runs[run.ID] = run
	return nil
}

func (r *memRepo) GetRun(_ context.Context, _ bun.IDB, id uuid.UUID) (*PipelineRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run, ok := r.runs[id]; ok {
		return run, nil
	}
	return nil, ErrNotFound
}

func (r *memRepo) ListRuns(_ context.Context, _ bun.IDB, f ListRunsFilters) ([]*PipelineRun, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*PipelineRun{}
	for _, run := range r.runs {
		if f.Pipeline != "" && run.PipelineName != f.Pipeline {
			continue
		}
		if len(f.Statuses) > 0 {
			ok := false
			for _, s := range f.Statuses {
				if run.Status == s {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, len(out), nil
}

func (r *memRepo) ListRunsByPipeline(_ context.Context, _ bun.IDB, name string, limit, offset int) ([]*PipelineRun, int, error) {
	return r.ListRuns(nil, nil, ListRunsFilters{Pipeline: name, Limit: limit, Offset: offset})
}

func (r *memRepo) FindInflightRun(_ context.Context, _ bun.IDB, name string, version int, hash string) (*PipelineRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.runs {
		if run.PipelineName == name && run.PipelineVersion == version && run.ContentHash == hash &&
			run.RetryOfRunID == nil && !run.Status.IsTerminal() {
			return run, nil
		}
	}
	return nil, nil
}

func (r *memRepo) UpdateRunStatusGuarded(_ context.Context, _ bun.IDB, id uuid.UUID, from []RunStatus, to RunStatus, f RunUpdateFields) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok {
		return 0, nil
	}
	allowed := false
	for _, s := range from {
		if run.Status == s {
			allowed = true
			break
		}
	}
	if !allowed {
		return 0, nil
	}
	run.Status = to
	if f.WorkerID != nil {
		v := *f.WorkerID
		run.WorkerID = &v
	} else if f.ClearWorkerID {
		run.WorkerID = nil
	}
	if f.LastHeartbeatAt != nil {
		t := *f.LastHeartbeatAt
		run.LastHeartbeatAt = &t
	} else if f.ClearHeartbeat {
		run.LastHeartbeatAt = nil
	}
	if f.StartedAt != nil {
		t := *f.StartedAt
		run.StartedAt = &t
	} else if f.ClearStartedAt {
		run.StartedAt = nil
	}
	if f.FinishedAt != nil {
		t := *f.FinishedAt
		run.FinishedAt = &t
	}
	if f.ErrorMsg != nil {
		v := *f.ErrorMsg
		run.Error = &v
	}
	run.UpdatedAt = time.Now().UTC()
	return 1, nil
}

func (r *memRepo) ScheduleAlreadyFired(_ context.Context, _ bun.IDB, name string, version int, firePoint time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.runs {
		if run.PipelineName == name && run.PipelineVersion == version &&
			run.TriggerSource == TriggerSchedule && !run.CreatedAt.Before(firePoint) {
			return true, nil
		}
	}
	return false, nil
}

func (r *memRepo) ClaimPendingRun(_ context.Context, _ bun.IDB, workerID string, now time.Time) (*PipelineRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var oldest *PipelineRun
	for _, run := range r.runs {
		if run.Status != RunPending {
			continue
		}
		if oldest == nil || run.CreatedAt.Before(oldest.CreatedAt) {
			oldest = run
		}
	}
	if oldest == nil {
		return nil, nil
	}
	oldest.Status = RunRunning
	wid := workerID
	oldest.WorkerID = &wid
	t := now
	oldest.StartedAt = &t
	oldest.LastHeartbeatAt = &t
	return oldest, nil
}

func (r *memRepo) RefreshHeartbeat(_ context.Context, _ bun.IDB, id uuid.UUID, workerID string, now time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok || run.Status != RunRunning || run.WorkerID == nil || *run.WorkerID != workerID {
		return 0, nil
	}
	t := now
	run.LastHeartbeatAt = &t
	return 1, nil
}

func (r *memRepo) ReclaimStaleRun(_ context.Context, _ bun.IDB, id uuid.UUID, threshold time.Duration) (*string, *time.Time, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok || run.Status != RunRunning {
		return nil, nil, false, nil
	}
	if run.LastHeartbeatAt != nil && time.Since(*run.LastHeartbeatAt) < threshold {
		return nil, nil, false, nil
	}
	prevWorker := run.WorkerID
	prevHB := run.LastHeartbeatAt
	run.Status = RunPending
	run.WorkerID = nil
	run.LastHeartbeatAt = nil
	run.StartedAt = nil
	return prevWorker, prevHB, true, nil
}

func (r *memRepo) ListStaleRunIDs(_ context.Context, _ bun.IDB, threshold time.Duration, limit int) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []uuid.UUID{}
	for _, run := range r.runs {
		if run.Status != RunRunning {
			continue
		}
		if run.LastHeartbeatAt == nil || time.Since(*run.LastHeartbeatAt) >= threshold {
			out = append(out, run.ID)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *memRepo) UpsertWorkerSlot(_ context.Context, _ bun.IDB, slot *WorkerSlot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.slots == nil {
		r.slots = map[string]*WorkerSlot{}
	}
	existing, ok := r.slots[slot.WorkerID]
	if ok {
		// UPSERT semantics: keep original started_at, only bump
		// last_heartbeat_at — mirrors the SQL implementation.
		existing.LastHeartbeatAt = slot.LastHeartbeatAt
		return nil
	}
	copyOf := *slot
	r.slots[slot.WorkerID] = &copyOf
	return nil
}

func (r *memRepo) DeleteWorkerSlot(_ context.Context, _ bun.IDB, workerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.slots, workerID)
	return nil
}

func (r *memRepo) ListWorkers(_ context.Context, _ bun.IDB, staleThreshold time.Duration) ([]WorkerSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().UTC().Add(-staleThreshold)
	// Pre-aggregate active runs per worker_id.
	type agg struct {
		count    int
		earliest *time.Time
	}
	active := map[string]*agg{}
	for _, run := range r.runs {
		if run.WorkerID == nil {
			continue
		}
		if run.Status != RunRunning && run.Status != RunCancelling {
			continue
		}
		a, ok := active[*run.WorkerID]
		if !ok {
			a = &agg{}
			active[*run.WorkerID] = a
		}
		a.count++
		if run.StartedAt != nil {
			if a.earliest == nil || run.StartedAt.Before(*a.earliest) {
				a.earliest = run.StartedAt
			}
		}
	}
	out := []WorkerSummary{}
	for _, slot := range r.slots {
		if slot.LastHeartbeatAt.Before(cutoff) {
			continue
		}
		s := WorkerSummary{
			WorkerID:        slot.WorkerID,
			Hostname:        slot.Hostname,
			PID:             slot.PID,
			SlotIndex:       slot.SlotIndex,
			StartedAt:       slot.StartedAt,
			LastHeartbeatAt: slot.LastHeartbeatAt,
		}
		if a, ok := active[slot.WorkerID]; ok {
			s.ActiveRuns = a.count
			s.EarliestRunStartAt = a.earliest
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkerID < out[j].WorkerID })
	return out, nil
}

func (r *memRepo) ListRunsByWorker(_ context.Context, _ bun.IDB, workerID string, limit, offset int) ([]*PipelineRun, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*PipelineRun{}
	for _, run := range r.runs {
		if run.WorkerID != nil && *run.WorkerID == workerID {
			out = append(out, run)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i].StartedAt, out[j].StartedAt
		if ai != nil && aj != nil {
			return ai.After(*aj)
		}
		return ai != nil
	})
	total := len(out)
	if offset > total {
		offset = total
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

// --- StepRun --------------------------------------------------------

func (r *memRepo) InsertStep(_ context.Context, _ bun.IDB, s *StepRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps[s.ID] = s
	return nil
}

func (r *memRepo) GetStep(_ context.Context, _ bun.IDB, id uuid.UUID) (*StepRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.steps[id]; ok {
		return s, nil
	}
	return nil, ErrNotFound
}

func (r *memRepo) LatestStepAttempt(_ context.Context, _ bun.IDB, runID uuid.UUID, stepName string, _ bool) (*StepRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *StepRun
	for _, s := range r.steps {
		if s.PipelineRunID != runID {
			continue
		}
		if stepName != "" && s.StepName != stepName {
			continue
		}
		if latest == nil || s.Attempt > latest.Attempt {
			latest = s
		}
	}
	if latest == nil {
		return nil, ErrNotFound
	}
	return latest, nil
}

func (r *memRepo) ListStepsByRun(_ context.Context, _ bun.IDB, runID uuid.UUID) ([]*StepRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*StepRun{}
	for _, s := range r.steps {
		if s.PipelineRunID == runID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *memRepo) UpdateStepStatusGuarded(_ context.Context, _ bun.IDB, id uuid.UUID, from []StepStatus, to StepStatus, f StepUpdateFields) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	step, ok := r.steps[id]
	if !ok {
		return 0, nil
	}
	allowed := false
	for _, s := range from {
		if step.Status == s {
			allowed = true
			break
		}
	}
	if !allowed {
		return 0, nil
	}
	step.Status = to
	if f.Result != nil {
		step.Result = f.Result
	}
	if f.ErrorMsg != nil {
		v := *f.ErrorMsg
		step.Error = &v
	}
	if f.FinishedAt != nil {
		t := *f.FinishedAt
		step.FinishedAt = &t
	}
	return 1, nil
}

func (r *memRepo) UpdateRunCurrentStep(_ context.Context, _ bun.IDB, id uuid.UUID, stepName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run, ok := r.runs[id]; ok {
		v := stepName
		run.CurrentStep = &v
	}
	return nil
}

func (r *memRepo) AbortRunningStepsForRun(_ context.Context, _ bun.IDB, runID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.steps {
		if s.PipelineRunID == runID && (s.Status == StepRunning || s.Status == StepAwaitingEvent) {
			s.Status = StepCancelled
		}
	}
	return nil
}

// --- EventWaiter ----------------------------------------------------

func (r *memRepo) InsertWaiter(_ context.Context, _ bun.IDB, w *EventWaiter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.waiters[w.StepRunID]; dup {
		return errors.New("uq_pipeline_event_waiters_step_run_id conflict")
	}
	r.waiters[w.StepRunID] = w
	return nil
}

func (r *memRepo) GetWaiterByStepRun(_ context.Context, _ bun.IDB, stepID uuid.UUID, _ bool) (*EventWaiter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.waiters[stepID]; ok {
		return w, nil
	}
	return nil, ErrNotFound
}

func (r *memRepo) DeleteWaiterByStepRun(_ context.Context, _ bun.IDB, stepID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.waiters, stepID)
	return nil
}

func (r *memRepo) FindMatchingWaiterStepIDs(_ context.Context, _ bun.IDB, eventType string, _ map[string]any) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []uuid.UUID{}
	for _, w := range r.waiters {
		if w.EventType == eventType && w.Status == WaiterWaiting {
			out = append(out, w.StepRunID)
		}
	}
	return out, nil
}

func (r *memRepo) ListExpiredWaiterStepIDs(_ context.Context, _ bun.IDB, now time.Time, _ int) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []uuid.UUID{}
	for _, w := range r.waiters {
		if w.ExpiresAt.Before(now) {
			out = append(out, w.StepRunID)
		}
	}
	return out, nil
}

// -------------------------------------------------------------------
// Actual tests
// -------------------------------------------------------------------

func newService() (*Service, *memRepo) {
	r := newMemRepo()
	return NewService(r), r
}

func TestCreateRun_FreshInsert(t *testing.T) {
	svc, _ := newService()
	res, err := svc.CreateRun(context.Background(), nil, CreateRunInput{
		PipelineName: "smoke.echo", PipelineVersion: 1,
		Args:          map[string]any{"x": 1},
		TriggerSource: TriggerHTTP,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if !res.Created || res.Run == nil || res.Run.Status != RunPending {
		t.Fatalf("Created=%v Status=%q", res.Created, res.Run.Status)
	}
}

func TestCreateRun_IdempotencyHit(t *testing.T) {
	svc, _ := newService()
	in := CreateRunInput{PipelineName: "smoke.echo", PipelineVersion: 1, Args: map[string]any{"x": 1}, TriggerSource: TriggerHTTP}

	first, err := svc.CreateRun(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateRun(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatalf("second insert should dedupe")
	}
	if second.Run.ID != first.Run.ID {
		t.Fatalf("dedupe returned different row id")
	}
}

func TestCreateRetry_OnlyTerminal(t *testing.T) {
	svc, repo := newService()
	id := uuid.New()
	repo.runs[id] = &PipelineRun{
		ID: id, PipelineName: "p", PipelineVersion: 1, Args: map[string]any{}, ContentHash: ContentHash(map[string]any{}),
		Status: RunRunning, TriggerSource: TriggerHTTP, CreatedAt: time.Now().UTC(),
	}
	_, err := svc.CreateRetry(context.Background(), nil, id)
	if err == nil {
		t.Fatalf("expected ErrNotRetryable on running")
	}
	var nr *ErrNotRetryable
	if !errors.As(err, &nr) {
		t.Fatalf("want *ErrNotRetryable, got %v", err)
	}

	repo.runs[id].Status = RunFailed
	res, err := svc.CreateRetry(context.Background(), nil, id)
	if err != nil {
		t.Fatalf("retry on terminal: %v", err)
	}
	if res.RetryOfRunID == nil || *res.RetryOfRunID != id {
		t.Fatalf("retry chain mis-pointing")
	}
	if res.TriggerSource != TriggerRetry {
		t.Fatalf("trigger_source = %s", res.TriggerSource)
	}
}

func TestRequestCancel_PendingSync(t *testing.T) {
	svc, repo := newService()
	id := uuid.New()
	repo.runs[id] = &PipelineRun{ID: id, Status: RunPending, CreatedAt: time.Now().UTC()}
	out, err := svc.RequestCancel(context.Background(), nil, id)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Sync || out.Status != RunCancelled {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestRequestCancel_RunningAsync(t *testing.T) {
	svc, repo := newService()
	id := uuid.New()
	repo.runs[id] = &PipelineRun{ID: id, Status: RunRunning, CreatedAt: time.Now().UTC()}
	out, err := svc.RequestCancel(context.Background(), nil, id)
	if err != nil {
		t.Fatal(err)
	}
	if out.Sync || out.Status != RunCancelling {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestRequestCancel_AlreadyCancelling(t *testing.T) {
	svc, repo := newService()
	id := uuid.New()
	repo.runs[id] = &PipelineRun{ID: id, Status: RunCancelling, CreatedAt: time.Now().UTC()}
	_, err := svc.RequestCancel(context.Background(), nil, id)
	if !errors.Is(err, ErrAlreadyCancelling) {
		t.Fatalf("want ErrAlreadyCancelling, got %v", err)
	}
}

func TestRequestCancel_Terminal(t *testing.T) {
	svc, repo := newService()
	id := uuid.New()
	repo.runs[id] = &PipelineRun{ID: id, Status: RunCompleted, CreatedAt: time.Now().UTC()}
	_, err := svc.RequestCancel(context.Background(), nil, id)
	if !errors.Is(err, ErrTerminal) {
		t.Fatalf("want ErrTerminal, got %v", err)
	}
}

func TestMarkRunCompleted_RaceCancelling(t *testing.T) {
	svc, repo := newService()
	id := uuid.New()
	// Run is now cancelling — MarkRunCompleted must silently transition
	// to cancelled.
	repo.runs[id] = &PipelineRun{ID: id, Status: RunCancelling, CreatedAt: time.Now().UTC()}
	if err := svc.MarkRunCompleted(context.Background(), nil, id); err != nil {
		t.Fatalf("MarkRunCompleted (race): %v", err)
	}
	if repo.runs[id].Status != RunCancelled {
		t.Fatalf("status = %s, want cancelled", repo.runs[id].Status)
	}
}

func TestResolveEventWaiter_CompletesStepAndReactivates(t *testing.T) {
	svc, repo := newService()
	runID := uuid.New()
	stepID := uuid.New()
	repo.runs[runID] = &PipelineRun{ID: runID, Status: RunAwaitingEvent, CreatedAt: time.Now().UTC()}
	repo.steps[stepID] = &StepRun{ID: stepID, PipelineRunID: runID, StepName: "park", Attempt: 1, Status: StepAwaitingEvent, CreatedAt: time.Now().UTC()}
	repo.waiters[stepID] = &EventWaiter{ID: uuid.New(), StepRunID: stepID, EventType: "approval.granted", Status: WaiterWaiting, ExpiresAt: time.Now().UTC().Add(time.Hour)}

	ok, err := svc.ResolveEventWaiter(context.Background(), nil, stepID, map[string]any{"reviewer": "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("ResolveEventWaiter = false")
	}
	if repo.steps[stepID].Status != StepCompleted {
		t.Fatalf("step status = %s", repo.steps[stepID].Status)
	}
	if repo.runs[runID].Status != RunPending {
		t.Fatalf("run status = %s, want pending after resolve", repo.runs[runID].Status)
	}
	if _, dup := repo.waiters[stepID]; dup {
		t.Fatalf("waiter not deleted")
	}
}

func TestListWorkers_RegistryJoinsActiveRuns(t *testing.T) {
	svc, repo := newService()
	wA := "host-1-101-0"
	wB := "host-1-101-1"
	wC := "host-1-101-2" // idle
	now := time.Now().UTC()
	hbFresh := now.Add(-1 * time.Second)
	startA1 := now.Add(-30 * time.Second)
	startA2 := now.Add(-10 * time.Second)
	startB := now.Add(-1 * time.Second)

	// Registry: three live slots — wA + wB busy, wC idle.
	for _, w := range []string{wA, wB, wC} {
		_ = svc.UpsertWorkerSlot(context.Background(), nil, &WorkerSlot{
			WorkerID: w, Hostname: "host-1", PID: 101,
			SlotIndex: 0, StartedAt: now.Add(-time.Minute), LastHeartbeatAt: hbFresh,
		})
	}
	// wA: 2 running runs (different start times)
	repo.runs[uuid.New()] = &PipelineRun{ID: uuid.New(), Status: RunRunning, WorkerID: &wA, StartedAt: &startA1}
	repo.runs[uuid.New()] = &PipelineRun{ID: uuid.New(), Status: RunRunning, WorkerID: &wA, StartedAt: &startA2}
	// wB: 1 cancelling run
	repo.runs[uuid.New()] = &PipelineRun{ID: uuid.New(), Status: RunCancelling, WorkerID: &wB, StartedAt: &startB}
	// Noise: completed + pending without worker — must not contribute to active_runs
	repo.runs[uuid.New()] = &PipelineRun{ID: uuid.New(), Status: RunCompleted, WorkerID: &wA}
	repo.runs[uuid.New()] = &PipelineRun{ID: uuid.New(), Status: RunPending}

	out, err := svc.ListWorkers(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("got %d workers, want 3 (including idle wC): %+v", len(out), out)
	}
	if out[0].WorkerID != wA || out[0].ActiveRuns != 2 {
		t.Fatalf("wA summary = %+v", out[0])
	}
	if out[0].EarliestRunStartAt == nil || !out[0].EarliestRunStartAt.Equal(startA1) {
		t.Fatalf("earliest start mismatch: %+v", out[0])
	}
	if out[1].WorkerID != wB || out[1].ActiveRuns != 1 {
		t.Fatalf("wB summary = %+v", out[1])
	}
	if out[2].WorkerID != wC || out[2].ActiveRuns != 0 || out[2].EarliestRunStartAt != nil {
		t.Fatalf("wC (idle) summary = %+v", out[2])
	}
}

func TestListWorkers_DropsStaleHeartbeats(t *testing.T) {
	svc, _ := newService()
	now := time.Now().UTC()
	// Fresh slot — within threshold.
	_ = svc.UpsertWorkerSlot(context.Background(), nil, &WorkerSlot{
		WorkerID: "fresh", Hostname: "h", PID: 1, SlotIndex: 0,
		StartedAt: now.Add(-time.Minute), LastHeartbeatAt: now.Add(-2 * time.Second),
	})
	// Stale slot — well past WorkerStaleThreshold (15s).
	_ = svc.UpsertWorkerSlot(context.Background(), nil, &WorkerSlot{
		WorkerID: "stale", Hostname: "h", PID: 1, SlotIndex: 1,
		StartedAt: now.Add(-time.Minute), LastHeartbeatAt: now.Add(-time.Minute),
	})

	out, err := svc.ListWorkers(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].WorkerID != "fresh" {
		t.Fatalf("expected fresh-only, got: %+v", out)
	}
}

func TestDeleteWorkerSlot_RemovesRegistryRow(t *testing.T) {
	svc, _ := newService()
	now := time.Now().UTC()
	_ = svc.UpsertWorkerSlot(context.Background(), nil, &WorkerSlot{
		WorkerID: "w-1", Hostname: "h", PID: 1, SlotIndex: 0,
		StartedAt: now, LastHeartbeatAt: now,
	})
	_ = svc.DeleteWorkerSlot(context.Background(), nil, "w-1")
	out, err := svc.ListWorkers(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty after delete, got: %+v", out)
	}
}

func TestListRunsByWorker_FiltersAndPaginates(t *testing.T) {
	svc, repo := newService()
	w := "host-1-101-0"
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		started := now.Add(time.Duration(-i) * time.Minute)
		repo.runs[uuid.New()] = &PipelineRun{
			ID: uuid.New(), Status: RunRunning, WorkerID: &w, StartedAt: &started,
		}
	}
	// Different worker — must not appear
	other := "other"
	repo.runs[uuid.New()] = &PipelineRun{ID: uuid.New(), Status: RunRunning, WorkerID: &other}

	items, total, err := svc.ListRunsByWorker(context.Background(), nil, w, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(items) != 2 {
		t.Fatalf("page size = %d, want 2", len(items))
	}
}

func TestClaimPendingRun_PickOldest(t *testing.T) {
	svc, repo := newService()
	older := &PipelineRun{ID: uuid.New(), Status: RunPending, CreatedAt: time.Now().UTC().Add(-time.Minute)}
	newer := &PipelineRun{ID: uuid.New(), Status: RunPending, CreatedAt: time.Now().UTC()}
	repo.runs[older.ID] = older
	repo.runs[newer.ID] = newer

	run, err := svc.ClaimPendingRun(context.Background(), nil, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.ID != older.ID {
		t.Fatalf("claimed %v, want older %v", run, older.ID)
	}
	if run.Status != RunRunning || run.WorkerID == nil || *run.WorkerID != "worker-1" {
		t.Fatalf("claim did not mark running: %+v", run)
	}
}

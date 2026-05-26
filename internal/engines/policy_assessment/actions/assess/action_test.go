// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package assess_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/engines/owner_assignment"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/actions/assess"
	opamech "github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/mechanisms/opa"
	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_assessment_runs"
	"github.com/aurelion-solutions/backplane/internal/inventory/principals"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/aurelion-solutions/backplane/internal/inventory/workloads"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// -------- in-memory fakes --------

type fakeAccountsRepo struct{ accs []*accounts.Account }

func (f *fakeAccountsRepo) Upsert(_ context.Context, _ bun.IDB, _ *accounts.Account) error {
	return nil
}
func (f *fakeAccountsRepo) SetDesiredState(_ context.Context, _ bun.IDB, _ uuid.UUID, _ string) error {
	return nil
}
func (f *fakeAccountsRepo) SetValidatedState(_ context.Context, _ bun.IDB, _ uuid.UUID, _ string) error {
	return nil
}
func (f *fakeAccountsRepo) SetEffectiveState(_ context.Context, _ bun.IDB, _ uuid.UUID, _ string) error {
	return nil
}
func (f *fakeAccountsRepo) List(_ context.Context, _ bun.IDB, _ accounts.ListFilter) ([]*accounts.Account, int, error) {
	return f.accs, len(f.accs), nil
}

type fakeWorkloadsRepo struct{ wls []*workloads.Workload }

func (f *fakeWorkloadsRepo) GetByID(_ context.Context, id uuid.UUID) (*workloads.Workload, error) {
	for _, w := range f.wls {
		if w.ID == id {
			return w, nil
		}
	}
	return nil, workloads.ErrNotFound
}
func (f *fakeWorkloadsRepo) GetByExternalID(_ context.Context, _ string) (*workloads.Workload, error) {
	return nil, workloads.ErrNotFound
}
func (f *fakeWorkloadsRepo) List(_ context.Context, _, _ int) ([]*workloads.Workload, int, error) {
	return f.wls, len(f.wls), nil
}
func (f *fakeWorkloadsRepo) ListByApplication(_ context.Context, _ uuid.UUID, _, _ int) ([]*workloads.Workload, int, error) {
	return f.wls, len(f.wls), nil
}
func (f *fakeWorkloadsRepo) Insert(_ context.Context, _ *workloads.Workload) error { return nil }
func (f *fakeWorkloadsRepo) Update(_ context.Context, _ *workloads.Workload) error { return nil }
func (f *fakeWorkloadsRepo) ListAttributes(_ context.Context, _ uuid.UUID) ([]*workloads.WorkloadAttribute, error) {
	return nil, nil
}
func (f *fakeWorkloadsRepo) GetAttribute(_ context.Context, _ uuid.UUID, _ string) (*workloads.WorkloadAttribute, error) {
	return nil, workloads.ErrAttributeNotFound
}
func (f *fakeWorkloadsRepo) UpsertAttribute(_ context.Context, _ *workloads.WorkloadAttribute) error {
	return nil
}
func (f *fakeWorkloadsRepo) DeleteAttribute(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (f *fakeWorkloadsRepo) BulkUpsert(_ context.Context, _ []workloads.BulkItem, _ func() uuid.UUID) (int, error) {
	return 0, nil
}

type fakePrincipalsRepo struct {
	byBody map[string]*principals.Principal // key: string(kind)+":"+bodyID.String()
}

func (f *fakePrincipalsRepo) GetByID(_ context.Context, _ uuid.UUID) (*principals.Principal, error) {
	return nil, principals.ErrNotFound
}
func (f *fakePrincipalsRepo) GetByBody(_ context.Context, kind shared.PrincipalKind, bodyID uuid.UUID) (*principals.Principal, error) {
	k := string(kind) + ":" + bodyID.String()
	if p, ok := f.byBody[k]; ok {
		return p, nil
	}
	return nil, principals.ErrNotFound
}
func (f *fakePrincipalsRepo) List(_ context.Context, _, _ int) ([]*principals.Principal, int, error) {
	return nil, 0, nil
}
func (f *fakePrincipalsRepo) Insert(_ context.Context, _ *principals.Principal) error { return nil }
func (f *fakePrincipalsRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _ string, _ time.Time) error {
	return nil
}
func (f *fakePrincipalsRepo) UpdateLock(_ context.Context, _ uuid.UUID, _ bool, _ time.Time) error {
	return nil
}
func (f *fakePrincipalsRepo) ListAttributes(_ context.Context, _ uuid.UUID) ([]*principals.PrincipalAttribute, error) {
	return nil, nil
}

type fakeRunsRepo struct {
	runs []*policy_assessment_runs.AssessmentRun
}

func (f *fakeRunsRepo) Insert(_ context.Context, r *policy_assessment_runs.AssessmentRun) error {
	f.runs = append(f.runs, r)
	return nil
}
func (f *fakeRunsRepo) Update(_ context.Context, r *policy_assessment_runs.AssessmentRun) error {
	for i, existing := range f.runs {
		if existing.ID == r.ID {
			f.runs[i] = r
		}
	}
	return nil
}
func (f *fakeRunsRepo) GetByID(_ context.Context, _ uuid.UUID) (*policy_assessment_runs.AssessmentRun, error) {
	return nil, nil
}
func (f *fakeRunsRepo) List(_ context.Context, _ policy_assessment_runs.ListFilter) ([]*policy_assessment_runs.AssessmentRun, int, error) {
	return f.runs, len(f.runs), nil
}

type fakeFindingsRepo struct {
	inserted []*findings.Finding
}

func (f *fakeFindingsRepo) Insert(_ context.Context, fin *findings.Finding) error {
	// Simulate the DB unique constraint on evidence_hash.
	for _, existing := range f.inserted {
		if existing.EvidenceHash == fin.EvidenceHash {
			return errors.New("SQLSTATE 23505: duplicate key value violates unique constraint uq_findings_evidence")
		}
	}
	f.inserted = append(f.inserted, fin)
	return nil
}
func (f *fakeFindingsRepo) GetByID(_ context.Context, _ uuid.UUID) (*findings.Finding, error) {
	return nil, nil
}
func (f *fakeFindingsRepo) List(_ context.Context, _ findings.ListFilter) ([]*findings.Finding, int, error) {
	return f.inserted, len(f.inserted), nil
}
func (f *fakeFindingsRepo) TouchLastSeen(_ context.Context, evidenceHash string, runID uuid.UUID, evaluatedAt time.Time) (int, error) {
	for _, existing := range f.inserted {
		if existing.EvidenceHash == evidenceHash {
			existing.LastSeenRunID = runID
			existing.EvaluatedAt = evaluatedAt
			return 1, nil
		}
	}
	return 0, nil
}
func (f *fakeFindingsRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _ string, _ *string) error {
	return nil
}

type fakeLineageResolver struct {
	chains map[uuid.UUID]assess.ResolvedChain
}

func (f *fakeLineageResolver) Resolve(_ context.Context, workloadID uuid.UUID) (assess.ResolvedChain, error) {
	if c, ok := f.chains[workloadID]; ok {
		return c, nil
	}
	return assess.ResolvedChain{}, errors.New("workload not found in fake resolver")
}

type fakeSnapshotWriter struct{ recorded []uuid.UUID }

func (f *fakeSnapshotWriter) RecordSnapshot(_ context.Context, workloadID uuid.UUID) error {
	f.recorded = append(f.recorded, workloadID)
	return nil
}

// -------- cartridge stub provider --------

// stubCartridgeProvider serves policies from an in-memory manifest map.
type stubCartridgeProvider struct {
	refs     []cartridges.Ref
	policies map[string]map[string]cartridges.Manifest
}

func (s *stubCartridgeProvider) List() ([]cartridges.Ref, error) { return s.refs, nil }
func (s *stubCartridgeProvider) Materialize(_ cartridges.Ref) (string, error) {
	return "", nil
}
func (s *stubCartridgeProvider) Policies(ref cartridges.Ref) (map[string]cartridges.Manifest, error) {
	return s.policies[ref.ID], nil
}
func (s *stubCartridgeProvider) Pipelines(_ cartridges.Ref) ([]string, error) { return nil, nil }
func (s *stubCartridgeProvider) Apps(_ cartridges.Ref) (map[string]cartridges.AppCartridge, error) {
	return nil, nil
}

// -------- OPA store builder --------

// writePolicyFiles writes a .meta.json + .rego pair into dir and returns the
// manifest with BasePath set to the meta file. The OPA handler derives the
// .rego path from BasePath (sibling with same base name).
func writePolicyFiles(t *testing.T, dir, ruleName, ruleID, regoBody string, tags []string, stackCheck *cartridges.StackCheck) cartridges.Manifest {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	metaPath := filepath.Join(dir, ruleName+".meta.json")
	if err := os.WriteFile(metaPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	regoPath := filepath.Join(dir, ruleName+".rego")
	if err := os.WriteFile(regoPath, []byte(regoBody), 0o644); err != nil {
		t.Fatalf("write rego: %v", err)
	}
	return cartridges.Manifest{
		RuleID:     ruleID,
		Version:    1,
		Name:       ruleName,
		Mechanism:  "opa",
		Tags:       tags,
		Severity:   "high",
		StackCheck: stackCheck,
		BasePath:   metaPath,
	}
}

// buildStoreAndDispatcher builds a Store + Dispatcher from a cartridge ref and manifests.
func buildStoreAndDispatcher(t *testing.T, cartridgeRef string, manifests []cartridges.Manifest) (*policy_assessment.Store, *policy_assessment.Dispatcher) {
	t.Helper()
	byID := make(map[string]cartridges.Manifest, len(manifests))
	for _, m := range manifests {
		byID[m.RuleID] = m
	}
	prov := &stubCartridgeProvider{
		refs:     []cartridges.Ref{{ID: cartridgeRef}},
		policies: map[string]map[string]cartridges.Manifest{cartridgeRef: byID},
	}
	store := policy_assessment.NewStore()
	if _, err := store.Reload(context.Background(), prov); err != nil {
		t.Fatalf("store reload: %v", err)
	}
	dispatcher := policy_assessment.NewDispatcher()
	dispatcher.Register(opamech.New())
	if ok, errs := dispatcher.PrepareAll(context.Background(), store.All()); len(errs) > 0 {
		t.Fatalf("prepare: %v (ok=%d)", errs, ok)
	}
	return store, dispatcher
}

// buildWorkloadPolicyStore loads a store+dispatcher for the workload terminated policy.
func buildWorkloadPolicyStore(t *testing.T) (*policy_assessment.Store, *policy_assessment.Dispatcher) {
	t.Helper()
	dir := t.TempDir()
	manifest := writePolicyFiles(t, dir,
		"workload_owned_by_terminated",
		"ispm_workload_posture.lifecycle.workload_owned_by_terminated",
		workloadTerminatedRego,
		[]string{"assessment", "scope:workload", "subject:workload"},
		&cartridges.StackCheck{Requires: []string{"subject_linkage", "ownership_resolved"}},
	)
	return buildStoreAndDispatcher(t, "ispm-workload-posture", []cartridges.Manifest{manifest})
}

// buildAccountPolicyStore loads a store+dispatcher for a simple account policy.
func buildAccountPolicyStore(t *testing.T) (*policy_assessment.Store, *policy_assessment.Dispatcher) {
	t.Helper()
	dir := t.TempDir()
	manifest := writePolicyFiles(t, dir,
		"terminated_access",
		"ispm_core.lifecycle.terminated_access",
		accountPolicyRego,
		[]string{"assessment", "scope:account", "resource:Account"},
		nil,
	)
	return buildStoreAndDispatcher(t, "ispm-core", []cartridges.Manifest{manifest})
}

const workloadTerminatedRego = `
package ispm_workload_posture.lifecycle.workload_owned_by_terminated
import future.keywords.if
default decision := null
decision := result if {
	object.get(input, ["subject","kind"], "") == "workload"
	object.get(input, ["subject","ownership","terminus"], "") == "terminated_human"
	result := {
		"risk_level": "high",
		"signals":    ["workload_owned_by_terminated"],
		"reasons":    [],
	}
}
`

const accountPolicyRego = `
package ispm_core.lifecycle.terminated_access
import future.keywords.if
default decision := null
decision := {"risk_level":"high","signals":["terminated_access"],"reasons":[]} if {
	input.target.kind == "account"
	input.target.account_status == "active"
}
`

func nopLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func actionCtx() registry.ActionContext {
	return registry.ActionContext{
		Ctx: context.Background(),
		Log: nopLog(),
	}
}

// emptyOwnerResolver returns a pre-built resolver with no app→owner mappings.
// Used in tests to avoid needing a real DB connection for owner_assignment.Load.
func emptyOwnerResolver() *owner_assignment.Resolver {
	return owner_assignment.NewResolver(nil)
}

// -------- tests --------

// TestWorkloadPass_TerminatedOwner_FindingCreated verifies that when the
// lineage resolver returns terminus=terminated_human, a finding of
// kind workload_owned_by_terminated is created keyed to the principal_id.
func TestWorkloadPass_TerminatedOwner_FindingCreated(t *testing.T) {
	store, dispatcher := buildWorkloadPolicyStore(t)

	wID := uuid.New()
	pID := uuid.New()
	appID := uuid.New()

	workload := &workloads.Workload{
		ID:            wID,
		ExternalID:    "svc-a",
		Name:          "Service A",
		ApplicationID: &appID,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	principal := &principals.Principal{
		ID:   pID,
		Kind: shared.PrincipalKindWorkload,
	}

	lineage := &fakeLineageResolver{
		chains: map[uuid.UUID]assess.ResolvedChain{
			wID: {
				WorkloadID:    wID,
				Terminus:      "terminated_human",
				OwnerPersonID: uuid.New().String(),
				OwnerLabel:    "John Doe",
			},
		},
	}
	snapshots := &fakeSnapshotWriter{}
	findingsRepo := &fakeFindingsRepo{}
	principalKey := string(shared.PrincipalKindWorkload) + ":" + wID.String()

	deps := assess.Deps{
		AccountsRepo:   &fakeAccountsRepo{},
		WorkloadsRepo:  &fakeWorkloadsRepo{wls: []*workloads.Workload{workload}},
		PrincipalsRepo: &fakePrincipalsRepo{byBody: map[string]*principals.Principal{principalKey: principal}},
		Lineage:        lineage,
		Snapshots:      snapshots,
		RunsRepo:       &fakeRunsRepo{},
		FindingsRepo:   findingsRepo,
		Store:          store,
		Dispatcher:     dispatcher,
		OwnerResolver:  emptyOwnerResolver(),
		Log:            nopLog(),
	}

	result, err := assess.New(deps)(assess.Args{TriggeredBy: "test"}, actionCtx())
	if err != nil {
		t.Fatalf("assess: %v", err)
	}

	// Finding must be created keyed to principal_id.
	if result.FindingsCreated == 0 {
		t.Errorf("want FindingsCreated > 0, got 0; kinds=%v", kindList(findingsRepo.inserted))
	}
	var workloadFinding *findings.Finding
	for _, f := range findingsRepo.inserted {
		if f.Kind == findings.KindWorkloadOwnedByTerminated {
			workloadFinding = f
			break
		}
	}
	if workloadFinding == nil {
		t.Fatalf("no workload_owned_by_terminated finding; kinds=%v", kindList(findingsRepo.inserted))
	}
	if workloadFinding.PrincipalID == nil || *workloadFinding.PrincipalID != pID {
		t.Errorf("want principal_id=%s, got %v", pID, workloadFinding.PrincipalID)
	}
	if workloadFinding.TargetType == nil || *workloadFinding.TargetType != findings.TargetWorkload {
		t.Errorf("Workload finding must set target_type=workload, got %v", workloadFinding.TargetType)
	}
	if workloadFinding.TargetID == nil || *workloadFinding.TargetID != wID {
		t.Errorf("Workload finding target_id must be the workload id %s, got %v", wID, workloadFinding.TargetID)
	}
	if workloadFinding.PriorityScore == 0 {
		t.Error("want non-zero priority_score")
	}
	if workloadFinding.CartridgeRef == nil {
		t.Error("want cartridge_ref set")
	}

	// Snapshot must have been written for the workload (R1: assess writes, GET does not).
	if len(snapshots.recorded) == 0 {
		t.Error("want snapshot recorded by assess pass, got none")
	}
	found := false
	for _, id := range snapshots.recorded {
		if id == wID {
			found = true
		}
	}
	if !found {
		t.Errorf("snapshot not recorded for workload %s; recorded=%v", wID, snapshots.recorded)
	}

	if result.WorkloadsEvaluated != 1 {
		t.Errorf("want workloads_evaluated=1, got %d", result.WorkloadsEvaluated)
	}
}

// TestWorkloadPass_BrokenLink_NotEvaluable verifies that when the chain
// is broken (ownership evidence absent), the policy produces a
// not_evaluable outcome (evidence_gap Blind Spot), not a silent not_matched.
func TestWorkloadPass_BrokenLink_NotEvaluable(t *testing.T) {
	store, dispatcher := buildWorkloadPolicyStore(t)

	wID := uuid.New()
	pID := uuid.New()
	appID := uuid.New()

	workload := &workloads.Workload{
		ID:            wID,
		ExternalID:    "svc-broken",
		Name:          "Broken Service",
		ApplicationID: &appID,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	principal := &principals.Principal{ID: pID, Kind: shared.PrincipalKindWorkload}

	lineage := &fakeLineageResolver{
		chains: map[uuid.UUID]assess.ResolvedChain{
			wID: {WorkloadID: wID, Terminus: "broken_link"},
		},
	}

	findingsRepo := &fakeFindingsRepo{}
	principalKey := string(shared.PrincipalKindWorkload) + ":" + wID.String()

	deps := assess.Deps{
		AccountsRepo:   &fakeAccountsRepo{},
		WorkloadsRepo:  &fakeWorkloadsRepo{wls: []*workloads.Workload{workload}},
		PrincipalsRepo: &fakePrincipalsRepo{byBody: map[string]*principals.Principal{principalKey: principal}},
		Lineage:        lineage,
		Snapshots:      &fakeSnapshotWriter{},
		RunsRepo:       &fakeRunsRepo{},
		FindingsRepo:   findingsRepo,
		Store:          store,
		Dispatcher:     dispatcher,
		OwnerResolver:  emptyOwnerResolver(),
		Log:            nopLog(),
	}

	result, err := assess.New(deps)(assess.Args{TriggeredBy: "test"}, actionCtx())
	if err != nil {
		t.Fatalf("assess: %v", err)
	}

	// Should produce not_evaluable (Blind Spot), not silent not_matched.
	if result.NotEvaluable == 0 {
		t.Error("want not_evaluable > 0 for broken_link chain")
	}
	var gapFinding *findings.Finding
	for _, f := range findingsRepo.inserted {
		if f.Kind == findings.KindEvidenceGap {
			gapFinding = f
			break
		}
	}
	if gapFinding == nil {
		t.Fatalf("want evidence_gap finding; kinds=%v", kindList(findingsRepo.inserted))
	}
	for _, f := range findingsRepo.inserted {
		if f.Kind == findings.KindWorkloadOwnedByTerminated {
			t.Error("workload_owned_by_terminated must NOT fire for broken_link")
		}
	}
}

// TestAccountPath_Idempotency_F2 verifies that a second assess run over
// an unchanged account population produces zero new findings.
// This is the F2 guardrail: targetRef generalisation must NOT perturb
// account-path evidence hashing byte-for-byte.
func TestAccountPath_Idempotency_F2(t *testing.T) {
	store, dispatcher := buildAccountPolicyStore(t)

	appID := uuid.New()
	accID := uuid.New()
	activeStatus := "active"
	acc := &accounts.Account{
		ID:            accID,
		ApplicationID: appID,
		Username:      "alice",
		IsActive:      true,
		Status:        &activeStatus,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	findingsRepo := &fakeFindingsRepo{}

	deps := assess.Deps{
		AccountsRepo:  &fakeAccountsRepo{accs: []*accounts.Account{acc}},
		WorkloadsRepo: &fakeWorkloadsRepo{},
		RunsRepo:      &fakeRunsRepo{},
		FindingsRepo:  findingsRepo,
		Store:         store,
		Dispatcher:    dispatcher,
		OwnerResolver: emptyOwnerResolver(),
		Log:           nopLog(),
	}

	// First run.
	r1, err := assess.New(deps)(assess.Args{}, actionCtx())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if r1.FindingsCreated == 0 {
		t.Fatal("F2: first run must create at least one finding")
	}
	firstHashes := make([]string, len(findingsRepo.inserted))
	for i, f := range findingsRepo.inserted {
		firstHashes[i] = f.EvidenceHash
	}

	// Second run over identical input.
	r2, err := assess.New(deps)(assess.Args{}, actionCtx())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if r2.FindingsCreated != 0 {
		t.Errorf("F2: second run created %d findings; want 0 (idempotency broken)", r2.FindingsCreated)
	}
	if r2.FindingsReused == 0 {
		t.Error("F2: second run must reuse existing findings")
	}
	// All hashes in repo must be unchanged (byte-identical to first run).
	for i, f := range findingsRepo.inserted[:len(firstHashes)] {
		if f.EvidenceHash != firstHashes[i] {
			t.Errorf("F2: finding[%d] evidence_hash changed: %s → %s", i, firstHashes[i], f.EvidenceHash)
		}
		// Reuse advances last_seen_run_id to the second run so a
		// current-posture view filtering on it still sees the finding,
		// while assessment_run_id stays pinned to first detection.
		if f.LastSeenRunID.String() != r2.AssessmentRunID {
			t.Errorf("F2: finding[%d] last_seen_run_id not advanced to second run: got %s want %s",
				i, f.LastSeenRunID, r2.AssessmentRunID)
		}
		if f.AssessmentRunID.String() != r1.AssessmentRunID {
			t.Errorf("F2: finding[%d] assessment_run_id drifted off first run: got %s want %s",
				i, f.AssessmentRunID, r1.AssessmentRunID)
		}
	}
}

// -------- helpers --------

func kindList(fs []*findings.Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Kind
	}
	return out
}

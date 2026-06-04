// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/inventory/evidence_chain"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
)

// --- fakes -----------------------------------------------------------

type fakeFindings struct {
	byID map[uuid.UUID]*findings.Finding
}

func (f fakeFindings) GetByID(_ context.Context, id uuid.UUID) (*findings.Finding, error) {
	if row, ok := f.byID[id]; ok {
		return row, nil
	}
	return nil, findings.ErrNotFound
}

type fakeEvidence struct {
	byFinding map[uuid.UUID][]*evidence_chain.EvidenceChain
}

func (f fakeEvidence) ListByFinding(_ context.Context, id uuid.UUID) ([]*evidence_chain.EvidenceChain, error) {
	return f.byFinding[id], nil
}

type fakeInference struct {
	out   string
	err   error
	calls int
}

func (f *fakeInference) Generate(_ context.Context, _ GenerateRequest) (GenerateResult, error) {
	f.calls++
	if f.err != nil {
		return GenerateResult{}, f.err
	}
	return GenerateResult{Output: f.out, TokensUsed: 7, ModelRef: "qwen-local:qwen2.5-7b-instruct"}, nil
}

type fakeRepo struct{ rows map[uuid.UUID]*Explanation }

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[uuid.UUID]*Explanation{}} }

func (r *fakeRepo) Insert(_ context.Context, row *Explanation) error {
	cp := *row
	r.rows[row.ID] = &cp
	return nil
}
func (r *fakeRepo) Update(_ context.Context, row *Explanation) error {
	cp := *row
	r.rows[row.ID] = &cp
	return nil
}
func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*Explanation, error) {
	if row, ok := r.rows[id]; ok {
		return row, nil
	}
	return nil, ErrExplanationNotFound
}
func (r *fakeRepo) GetByFindingAndHash(_ context.Context, fid uuid.UUID, hash string) (*Explanation, error) {
	for _, row := range r.rows {
		if row.FindingID == fid && row.InputHash == hash {
			return row, nil
		}
	}
	return nil, ErrExplanationNotFound
}
func (r *fakeRepo) GetLatestByFinding(_ context.Context, fid uuid.UUID) (*Explanation, error) {
	var latest *Explanation
	for _, row := range r.rows {
		if row.FindingID == fid {
			if latest == nil || row.CreatedAt.After(latest.CreatedAt) {
				latest = row
			}
		}
	}
	if latest == nil {
		return nil, ErrExplanationNotFound
	}
	return latest, nil
}

// --- fixtures --------------------------------------------------------

func newFinding() *findings.Finding {
	id := uuid.New()
	run := uuid.New()
	pol := uuid.New()
	return &findings.Finding{
		ID:              id,
		AssessmentRunID: run,
		LastSeenRunID:   run,
		Kind:            "privileged_access",
		Severity:        "high",
		Status:          "open",
		PolicyID:        &pol,
	}
}

func buildService(out string, infErr error, f *findings.Finding, chains []*evidence_chain.EvidenceChain) (*Service, *fakeInference, *fakeRepo) {
	inf := &fakeInference{out: out, err: infErr}
	repo := newFakeRepo()
	svc := NewService(Deps{
		Findings:  fakeFindings{byID: map[uuid.UUID]*findings.Finding{f.ID: f}},
		Evidence:  fakeEvidence{byFinding: map[uuid.UUID][]*evidence_chain.EvidenceChain{f.ID: chains}},
		Inference: inf,
		Repo:      repo,
	})
	return svc, inf, repo
}

// --- tests -----------------------------------------------------------

func TestExplainHappyPath(t *testing.T) {
	f := newFinding()
	svc, inf, repo := buildService("Account [F] lacks MFA per policy [P1].", nil, f, nil)

	view, err := svc.Explain(context.Background(), f.ID, ExplainRequest{})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if view.Status != StatusCompleted {
		t.Fatalf("status = %q, want completed", view.Status)
	}
	if view.ModelRef != "qwen-local:qwen2.5-7b-instruct" {
		t.Fatalf("model_ref = %q", view.ModelRef)
	}
	// [F] and [P1] are valid refs; both should be cited.
	if len(view.Citations) != 2 {
		t.Fatalf("citations = %d, want 2 (%+v)", len(view.Citations), view.Citations)
	}
	if inf.calls != 1 {
		t.Fatalf("inference calls = %d, want 1", inf.calls)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("persisted rows = %d, want 1", len(repo.rows))
	}
}

func TestExplainCacheHitSkipsInference(t *testing.T) {
	f := newFinding()
	svc, inf, _ := buildService("Account [F] flagged.", nil, f, nil)

	if _, err := svc.Explain(context.Background(), f.ID, ExplainRequest{}); err != nil {
		t.Fatalf("first Explain: %v", err)
	}
	if _, err := svc.Explain(context.Background(), f.ID, ExplainRequest{}); err != nil {
		t.Fatalf("second Explain: %v", err)
	}
	if inf.calls != 1 {
		t.Fatalf("inference calls = %d, want 1 (cache hit on second)", inf.calls)
	}
}

func TestExplainForceRegenerates(t *testing.T) {
	f := newFinding()
	svc, inf, _ := buildService("Account [F] flagged.", nil, f, nil)

	if _, err := svc.Explain(context.Background(), f.ID, ExplainRequest{}); err != nil {
		t.Fatalf("first Explain: %v", err)
	}
	if _, err := svc.Explain(context.Background(), f.ID, ExplainRequest{Force: true}); err != nil {
		t.Fatalf("forced Explain: %v", err)
	}
	if inf.calls != 2 {
		t.Fatalf("inference calls = %d, want 2 (force regenerates)", inf.calls)
	}
}

func TestExplainFindingNotFound(t *testing.T) {
	f := newFinding()
	svc, _, _ := buildService("x", nil, f, nil)
	_, err := svc.Explain(context.Background(), uuid.New(), ExplainRequest{})
	if !errors.Is(err, ErrFindingNotFound) {
		t.Fatalf("err = %v, want ErrFindingNotFound", err)
	}
}

func TestExplainGenerationFailurePersistsFailed(t *testing.T) {
	f := newFinding()
	svc, _, repo := buildService("", errors.New("gateway 501"), f, nil)

	_, err := svc.Explain(context.Background(), f.ID, ExplainRequest{})
	if !errors.Is(err, ErrGenerationFailed) {
		t.Fatalf("err = %v, want ErrGenerationFailed", err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("a failed artifact should be persisted, rows = %d", len(repo.rows))
	}
	for _, row := range repo.rows {
		if row.Status != StatusFailed || row.Error == nil {
			t.Fatalf("persisted row = %+v, want failed with error", row)
		}
	}
}

func TestLatestReturnsNewest(t *testing.T) {
	f := newFinding()
	svc, _, _ := buildService("Account [F] flagged.", nil, f, nil)
	if _, err := svc.Explain(context.Background(), f.ID, ExplainRequest{}); err != nil {
		t.Fatalf("Explain: %v", err)
	}
	view, err := svc.Latest(context.Background(), f.ID)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if view.FindingID != f.ID || view.Status != StatusCompleted {
		t.Fatalf("latest = %+v", view)
	}
}

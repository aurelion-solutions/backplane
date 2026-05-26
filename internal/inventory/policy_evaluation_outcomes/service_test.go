// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_evaluation_outcomes

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeRepo captures Upsert calls and serves canned List/GetByID.
type fakeRepo struct {
	upserts []*PolicyEvaluationOutcome
	list    []*PolicyEvaluationOutcome
	upErr   error
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*PolicyEvaluationOutcome, error) {
	for _, o := range f.upserts {
		if o.ID == id {
			return o, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) List(_ context.Context, _ ListFilter) ([]*PolicyEvaluationOutcome, int, error) {
	return f.list, len(f.list), nil
}

func (f *fakeRepo) Upsert(_ context.Context, o *PolicyEvaluationOutcome) error {
	if f.upErr != nil {
		return f.upErr
	}
	f.upserts = append(f.upserts, o)
	return nil
}

func baseParams() RecordParams {
	ref := uuid.New()
	return RecordParams{
		AssessmentRunID: uuid.New(),
		CartridgeID:     "ispm-core-identity-posture",
		RuleID:          "ispm_core_identity_posture.lifecycle.terminated_subject_access",
		TargetType:      TargetAccount,
		TargetRef:       &ref,
		TargetKey:       "alice@corp",
		Outcome:         OutcomeMatched,
	}
}

func TestRecordOutcome_BiconditionalHolds(t *testing.T) {
	cases := []struct {
		name    string
		outcome string
		missing []string
		wantErr bool
	}{
		{"matched no missing", OutcomeMatched, nil, false},
		{"not_matched no missing", OutcomeNotMatched, nil, false},
		{"not_evaluable with missing", OutcomeNotEvaluable, []string{"mfa_evidence"}, false},
		{"not_evaluable empty missing -> reject", OutcomeNotEvaluable, nil, true},
		{"matched with missing -> reject", OutcomeMatched, []string{"mfa_evidence"}, true},
		{"not_matched with missing -> reject", OutcomeNotMatched, []string{"x"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := NewService(repo)
			p := baseParams()
			p.Outcome = tc.outcome
			p.MissingEvidence = tc.missing
			_, err := svc.RecordOutcome(context.Background(), p)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidOutcome) {
					t.Fatalf("want ErrInvalidOutcome, got %v", err)
				}
				if len(repo.upserts) != 0 {
					t.Fatalf("rejected outcome must not persist, got %d upserts", len(repo.upserts))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repo.upserts) != 1 {
				t.Fatalf("want 1 upsert, got %d", len(repo.upserts))
			}
		})
	}
}

func TestRecordOutcome_RejectsUnknownEnums(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	bad := baseParams()
	bad.Outcome = "weird"
	if _, err := svc.RecordOutcome(context.Background(), bad); !errors.Is(err, ErrInvalidOutcome) {
		t.Fatalf("unknown outcome: want ErrInvalidOutcome, got %v", err)
	}

	bad = baseParams()
	bad.TargetType = "server"
	if _, err := svc.RecordOutcome(context.Background(), bad); !errors.Is(err, ErrInvalidOutcome) {
		t.Fatalf("unknown target_type: want ErrInvalidOutcome, got %v", err)
	}
}

func TestRecordOutcome_RequiresAnchors(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	p := baseParams()
	p.AssessmentRunID = uuid.Nil
	if _, err := svc.RecordOutcome(context.Background(), p); !errors.Is(err, ErrInvalidOutcome) {
		t.Fatalf("nil run id: want ErrInvalidOutcome, got %v", err)
	}
}

func TestRecordOutcome_NormalisesNilMissingToEmptySlice(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	got, err := svc.RecordOutcome(context.Background(), baseParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MissingEvidence == nil {
		t.Fatal("MissingEvidence must be non-nil empty slice for jsonb notnull")
	}
	if got.ID == uuid.Nil {
		t.Fatal("service must assign an ID")
	}
}

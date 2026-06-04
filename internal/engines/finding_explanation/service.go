// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
)

// Deps are the collaborators a Service needs — all ports, so the engine
// stays testable without a database or a live gateway.
type Deps struct {
	Findings  FindingsReader
	Evidence  EvidenceReader
	Inference InferenceClient
	Repo      Repository
	Log       *slog.Logger
}

// Service orchestrates one explanation: collect context, hash, reuse or
// generate via the gateway, validate citations, persist.
type Service struct {
	deps Deps
}

// NewService constructs a Service.
func NewService(deps Deps) *Service {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Service{deps: deps}
}

// Explain returns an explanation for a finding, generating one unless a
// fresh cached explanation already exists (and Force is not set).
func (s *Service) Explain(ctx context.Context, findingID uuid.UUID, req ExplainRequest) (ExplanationView, error) {
	finding, err := s.deps.Findings.GetByID(ctx, findingID)
	if err != nil {
		if errors.Is(err, findings.ErrNotFound) {
			return ExplanationView{}, ErrFindingNotFound
		}
		return ExplanationView{}, err
	}

	chains, err := s.deps.Evidence.ListByFinding(ctx, findingID)
	if err != nil {
		return ExplanationView{}, err
	}

	cctx := collectContext(finding, chains)
	ih := inputHash(cctx, req.Provider, req.Language)

	// Cache: reuse a completed explanation for the same inputs.
	existing, err := s.deps.Repo.GetByFindingAndHash(ctx, findingID, ih)
	if err != nil && !errors.Is(err, ErrExplanationNotFound) {
		return ExplanationView{}, err
	}
	if existing != nil && existing.Status == StatusCompleted && !req.Force {
		return existing.view(), nil
	}

	row := existing
	isNew := row == nil
	if isNew {
		row = &Explanation{
			ID:                    uuid.New(),
			FindingID:             finding.ID,
			AssessmentRunID:       finding.LastSeenRunID,
			PolicyID:              finding.PolicyID,
			InputHash:             ih,
			PromptTemplateVersion: promptTemplateVersion,
			Status:                StatusRunning,
			Citations:             []Citation{},
			Refs:                  cctx.refs,
			CreatedAt:             time.Now().UTC(),
		}
	}

	res, gErr := s.deps.Inference.Generate(ctx, GenerateRequest{
		Provider: req.Provider,
		Messages: renderMessages(cctx, req.Language),
	})
	now := time.Now().UTC()
	if gErr != nil {
		msg := gErr.Error()
		row.Status = StatusFailed
		row.Error = &msg
		row.CompletedAt = &now
		if perr := s.persist(ctx, row, isNew); perr != nil {
			return ExplanationView{}, perr
		}
		s.deps.Log.Warn("explanation generation failed",
			slog.String("finding_id", findingID.String()),
			slog.Any("err", gErr))
		return ExplanationView{}, ErrGenerationFailed
	}

	cited, stray := validateCitations(res.Output, cctx.refs)
	if len(stray) > 0 {
		// Strays are dropped, not fatal — the narrative kept claims the
		// model could not anchor. Surfacing them helps tune the prompt.
		s.deps.Log.Info("explanation dropped stray citations",
			slog.String("finding_id", findingID.String()),
			slog.Any("labels", stray))
	}

	row.Status = StatusCompleted
	row.Narrative = res.Output
	row.Citations = cited
	row.ModelRef = res.ModelRef
	row.Error = nil
	row.CompletedAt = &now
	if perr := s.persist(ctx, row, isNew); perr != nil {
		return ExplanationView{}, perr
	}
	return row.view(), nil
}

// Latest returns the most recent explanation for a finding.
func (s *Service) Latest(ctx context.Context, findingID uuid.UUID) (ExplanationView, error) {
	row, err := s.deps.Repo.GetLatestByFinding(ctx, findingID)
	if err != nil {
		return ExplanationView{}, err
	}
	return row.view(), nil
}

// Get returns one explanation by its id (the explanation-job handle).
func (s *Service) Get(ctx context.Context, id uuid.UUID) (ExplanationView, error) {
	row, err := s.deps.Repo.GetByID(ctx, id)
	if err != nil {
		return ExplanationView{}, err
	}
	return row.view(), nil
}

// persist inserts a new row or updates an existing one.
func (s *Service) persist(ctx context.Context, row *Explanation, isNew bool) error {
	if isNew {
		return s.deps.Repo.Insert(ctx, row)
	}
	return s.deps.Repo.Update(ctx, row)
}

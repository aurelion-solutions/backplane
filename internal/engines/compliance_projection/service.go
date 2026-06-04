// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_assessment_runs"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
)

// Service computes compliance projections over an assessment run. It is
// read-only and stateless beyond its injected readers — every call
// recomputes from the run's findings and outcomes.
type Service struct {
	cartridges CartridgeReader
	findings   FindingsReader
	outcomes   OutcomesReader
	runs       RunReader
}

// NewService wires the projection service to its read collaborators.
func NewService(c CartridgeReader, f FindingsReader, o OutcomesReader, r RunReader) (*Service, error) {
	if c == nil || f == nil || o == nil || r == nil {
		return nil, errors.New("compliance_projection: all readers are required")
	}
	return &Service{cartridges: c, findings: f, outcomes: o, runs: r}, nil
}

// runContext is everything the coverage computation reads from a run,
// fetched once and shared across controls.
type runContext struct {
	period         Period
	findingsByKind map[string]int
	outcomes       []*policy_evaluation_outcomes.PolicyEvaluationOutcome
	ruleKind       map[string]string
}

// Projections lists every projection available for a run with a coverage
// roll-up per projection.
func (s *Service) Projections(ctx context.Context, runID uuid.UUID) ([]ProjectionSummary, error) {
	rc, err := s.loadRunContext(ctx, runID)
	if err != nil {
		return nil, err
	}
	defs, err := loadDefinitions(s.cartridges)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectionSummary, 0, len(defs))
	for _, def := range defs {
		_, summary := computeCoverage(def, rc.findingsByKind, rc.outcomes, rc.ruleKind)
		out = append(out, ProjectionSummary{
			Projection: def.Projection,
			Name:       def.Name,
			Type:       def.Type,
			Summary:    summary,
		})
	}
	return out, nil
}

// Coverage returns the per-control coverage table for one projection.
func (s *Service) Coverage(ctx context.Context, runID uuid.UUID, projection string) (CoverageReport, error) {
	def, err := loadDefinition(s.cartridges, projection)
	if err != nil {
		return CoverageReport{}, err
	}
	rc, err := s.loadRunContext(ctx, runID)
	if err != nil {
		return CoverageReport{}, err
	}
	controls, summary := computeCoverage(def, rc.findingsByKind, rc.outcomes, rc.ruleKind)
	return CoverageReport{
		Projection:     def.Projection,
		Name:           def.Name,
		Type:           def.Type,
		CriteriaSource: def.CriteriaSource,
		Disclaimer:     def.Disclaimer,
		AssessmentRun:  runID,
		Period:         rc.period,
		Summary:        summary,
		Controls:       controls,
	}, nil
}

// ControlDetail returns one control's state plus the supporting findings
// and the blind spots holding it back.
func (s *Service) ControlDetail(ctx context.Context, runID uuid.UUID, projection, controlID string) (ControlDetail, error) {
	def, err := loadDefinition(s.cartridges, projection)
	if err != nil {
		return ControlDetail{}, err
	}
	control, ok := def.control(controlID)
	if !ok {
		return ControlDetail{}, fmt.Errorf("%w: %q in %q", ErrControlNotFound, controlID, projection)
	}
	rc, err := s.loadRunContext(ctx, runID)
	if err != nil {
		return ControlDetail{}, err
	}

	cc := computeControl(control, gatherControlInputs(control, rc.findingsByKind, rc.outcomes, rc.ruleKind))

	violations, err := s.violationRefs(ctx, runID, control)
	if err != nil {
		return ControlDetail{}, err
	}
	gaps := s.gapRefs(control, rc.outcomes, rc.ruleKind)

	return ControlDetail{
		Projection:    def.Projection,
		AssessmentRun: runID,
		Period:        rc.period,
		Control:       control,
		State:         cc.State,
		Violations:    violations,
		Gaps:          gaps,
	}, nil
}

// loadRunContext fetches the run period, the findings-by-kind tally, the
// run's outcomes, and the rule→kind map once per request.
func (s *Service) loadRunContext(ctx context.Context, runID uuid.UUID) (runContext, error) {
	run, err := s.runs.GetByID(ctx, runID)
	if err != nil {
		return runContext{}, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}

	byKind, err := s.findingsByKind(ctx, runID)
	if err != nil {
		return runContext{}, err
	}
	outcomes, _, err := s.outcomes.List(ctx, policy_evaluation_outcomes.ListFilter{AssessmentRunID: &runID})
	if err != nil {
		return runContext{}, fmt.Errorf("compliance_projection: list outcomes: %w", err)
	}
	ruleKind, err := s.ruleKindMap()
	if err != nil {
		return runContext{}, err
	}
	return runContext{
		period:         periodFromRun(run),
		findingsByKind: byKind,
		outcomes:       outcomes,
		ruleKind:       ruleKind,
	}, nil
}

// findingsByKind tallies the run's current-posture findings by kind.
func (s *Service) findingsByKind(ctx context.Context, runID uuid.UUID) (map[string]int, error) {
	list, _, err := s.findings.List(ctx, findings.ListFilter{LastSeenRunID: &runID})
	if err != nil {
		return nil, fmt.Errorf("compliance_projection: list findings: %w", err)
	}
	out := map[string]int{}
	for _, f := range list {
		out[f.Kind]++
	}
	return out, nil
}

// violationRefs returns thin references to the baseline findings that
// violate a control.
func (s *Service) violationRefs(ctx context.Context, runID uuid.UUID, c Control) ([]FindingRef, error) {
	refs := []FindingRef{}
	for _, kind := range c.ViolatingKinds {
		list, _, err := s.findings.List(ctx, findings.ListFilter{LastSeenRunID: &runID, Kind: kind})
		if err != nil {
			return nil, fmt.Errorf("compliance_projection: list findings %q: %w", kind, err)
		}
		for _, f := range list {
			refs = append(refs, FindingRef{
				ID:         f.ID,
				Kind:       f.Kind,
				Severity:   f.Severity,
				Status:     f.Status,
				TargetType: f.TargetType,
				TargetID:   f.TargetID,
				ScopeValue: f.ScopeValue,
			})
		}
	}
	return refs, nil
}

// gapRefs returns the not_evaluable outcomes (blind spots) on rules a
// control depends on.
func (s *Service) gapRefs(c Control, outcomes []*policy_evaluation_outcomes.PolicyEvaluationOutcome, ruleKind map[string]string) []GapRef {
	violating := make(map[string]struct{}, len(c.ViolatingKinds))
	for _, k := range c.ViolatingKinds {
		violating[k] = struct{}{}
	}
	refs := []GapRef{}
	for _, o := range outcomes {
		if o.Outcome != policy_evaluation_outcomes.OutcomeNotEvaluable {
			continue
		}
		kind, ok := ruleKind[o.RuleID]
		if !ok {
			continue
		}
		if _, relevant := violating[kind]; !relevant {
			continue
		}
		refs = append(refs, GapRef{
			RuleID:          o.RuleID,
			Kind:            kind,
			TargetType:      o.TargetType,
			TargetKey:       o.TargetKey,
			MissingEvidence: o.MissingEvidence,
		})
	}
	return refs
}

// ruleKindMap builds rule_id → finding kind across every cartridge so
// the engine can attribute an outcome to the control it serves. The
// engine never duplicates a policy; it only reads the kind a policy
// already declares.
func (s *Service) ruleKindMap() (map[string]string, error) {
	refs, err := s.cartridges.List()
	if err != nil {
		return nil, fmt.Errorf("compliance_projection: list cartridges: %w", err)
	}
	out := map[string]string{}
	for _, ref := range refs {
		policies, err := s.cartridges.Policies(ref)
		if err != nil {
			return nil, fmt.Errorf("compliance_projection: policies %q: %w", ref.ID, err)
		}
		for ruleID, m := range policies {
			if m.Finding != nil && m.Finding.Kind != "" {
				out[ruleID] = m.Finding.Kind
			}
		}
	}
	return out, nil
}

// periodFromRun maps a run's temporal anchors to the projection period.
func periodFromRun(run *policy_assessment_runs.AssessmentRun) Period {
	return Period{
		AsOf:  run.AsOf,
		Start: run.PeriodStart,
		End:   run.PeriodEnd,
	}
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package assess

import (
	"context"
	"fmt"
	"time"

	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
	"github.com/google/uuid"
)

// Coverage `check` discriminators carried in a coverage policy's
// manifest body. The cartridge owns which checks exist and their
// thresholds; this engine owns how each aggregate is computed.
const (
	checkSourceStale           = "source_stale"
	checkRawNotNormalised      = "raw_not_normalised"
	checkNormalisedNotResolved = "normalised_not_resolved"
)

// runCoverageChecks evaluates the aggregate source/pipeline coverage
// policies (mechanism: coverage) once per run, after the per-account
// loop. Unlike per-target OPA policies these answer estate-level
// questions — "is this source stale?", "did raw rows normalise?",
// "did normalised facts resolve to effective access?" — and emit PEO
// rows with target_type source / pipeline (no per-account finding).
func runCoverageChecks(ctx context.Context, deps Deps, res *Result, runID uuid.UUID, cartridgeAllow map[string]struct{}) error {
	if deps.OutcomesSvc == nil {
		return nil
	}
	for _, entry := range deps.Store.SelectByMechanism("coverage") {
		if cartridgeAllow != nil {
			if _, ok := cartridgeAllow[entry.CartridgeRef]; !ok {
				continue
			}
		}
		check, _ := entry.Manifest.Body["check"].(string)
		var err error
		switch check {
		case checkSourceStale:
			err = coverageSourceStale(ctx, deps, res, runID, entry)
		case checkRawNotNormalised:
			err = coverageRawNotNormalised(ctx, deps, res, runID, entry)
		case checkNormalisedNotResolved:
			err = coverageNormalisedNotResolved(ctx, deps, res, runID, entry)
		default:
			// Unknown coverage check — skip rather than fail the run.
			continue
		}
		if err != nil {
			return fmt.Errorf("coverage %s/%s: %w", entry.CartridgeRef, entry.Manifest.RuleID, err)
		}
	}
	return nil
}

// coverageSourceStale flags each source whose most recent ingest is
// older than the manifest's max_staleness_hours.
func coverageSourceStale(ctx context.Context, deps Deps, res *Result, runID uuid.UUID, entry policy_assessment.Entry) error {
	maxHours := bodyFloat(entry.Manifest.Body, "max_staleness_hours", 24)
	var rows []struct {
		Source   string    `bun:"source"`
		LastSeen time.Time `bun:"last_seen"`
	}
	if err := deps.DB.NewSelect().
		TableExpr("inventory_ingest_batches").
		ColumnExpr("source").
		ColumnExpr("max(completed_at) AS last_seen").
		GroupExpr("source").
		Scan(ctx, &rows); err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(maxHours) * time.Hour)
	for _, r := range rows {
		stale := r.LastSeen.Before(cutoff)
		if err := emitCoverage(ctx, deps, res, runID, entry, policy_evaluation_outcomes.TargetSource, r.Source, stale, "fresh_source_data"); err != nil {
			return err
		}
	}
	return nil
}

// coverageRawNotNormalised flags each source where ingested raw rows
// exceeded the rows written into the normalised tables.
func coverageRawNotNormalised(ctx context.Context, deps Deps, res *Result, runID uuid.UUID, entry policy_assessment.Entry) error {
	var rows []struct {
		Source   string `bun:"source"`
		Received int    `bun:"received"`
		Written  int    `bun:"written"`
	}
	if err := deps.DB.NewSelect().
		TableExpr("inventory_ingest_batches").
		ColumnExpr("source").
		ColumnExpr("sum(received_count) AS received").
		ColumnExpr("sum(written_count) AS written").
		GroupExpr("source").
		Scan(ctx, &rows); err != nil {
		return err
	}
	for _, r := range rows {
		gap := r.Received > r.Written
		if err := emitCoverage(ctx, deps, res, runID, entry, policy_evaluation_outcomes.TargetPipeline, r.Source+":normalise", gap, "normalised_records"); err != nil {
			return err
		}
	}
	return nil
}

// coverageNormalisedNotResolved flags the accounts→effective-access
// stage when normalised accounts have no resolved capability grant.
func coverageNormalisedNotResolved(ctx context.Context, deps Deps, res *Result, runID uuid.UUID, entry policy_assessment.Entry) error {
	count, err := deps.DB.NewSelect().
		TableExpr("accounts AS a").
		Join("LEFT JOIN capability_grants AS g ON g.account_id = a.id").
		Where("g.id IS NULL").
		Count(ctx)
	if err != nil {
		return err
	}
	return emitCoverage(ctx, deps, res, runID, entry, policy_evaluation_outcomes.TargetPipeline, "account→effective_access", count > 0, "effective_access_resolution")
}

// emitCoverage records one coverage outcome: not_evaluable + the
// missing-evidence key when the gap is present, not_matched otherwise.
func emitCoverage(
	ctx context.Context, deps Deps, res *Result, runID uuid.UUID,
	entry policy_assessment.Entry, targetType, targetKey string, gap bool, missingKey string,
) error {
	outcome := policy_evaluation_outcomes.OutcomeNotMatched
	var missing []string
	if gap {
		outcome = policy_evaluation_outcomes.OutcomeNotEvaluable
		missing = []string{missingKey}
	}
	if _, err := deps.OutcomesSvc.RecordOutcome(ctx, policy_evaluation_outcomes.RecordParams{
		AssessmentRunID: runID,
		CartridgeID:     entry.CartridgeRef,
		RuleID:          entry.Manifest.RuleID,
		TargetType:      targetType,
		TargetKey:       targetKey,
		Outcome:         outcome,
		MissingEvidence: missing,
	}); err != nil {
		return err
	}
	if gap {
		res.NotEvaluable++
	} else {
		res.NotMatched++
	}
	return nil
}

func bodyFloat(body map[string]any, key string, def float64) float64 {
	if v, ok := body[key].(float64); ok {
		return v
	}
	return def
}

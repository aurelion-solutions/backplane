// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package assess

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_assessment_runs"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Deps is the composition-root injection set.
//
// AccountsRepo provides the population snapshot via List. Store +
// Dispatcher are the in-memory engine references that the worker
// composition root populates and keeps refreshed via the cartridge
// watcher. RunsRepo / FindingsRepo persist the action's output.
type Deps struct {
	DB           *bun.DB
	AccountsRepo accounts.Repository
	RunsRepo     policy_assessment_runs.Repository
	FindingsRepo findings.Repository
	Store        *policy_assessment.Store
	Dispatcher   *policy_assessment.Dispatcher
}

// New builds the handler closure with deps bound.
func New(deps Deps) registry.Handler[Args, Result] {
	return func(args Args, ctx registry.ActionContext) (Result, error) {
		now := time.Now().UTC()
		trig := args.TriggeredBy
		if trig == "" {
			trig = policy_assessment_runs.TriggerSchedule
		}
		run := &policy_assessment_runs.AssessmentRun{
			ID:                 uuid.New(),
			Status:             policy_assessment_runs.StatusRunning,
			TriggeredBy:        trig,
			StartedAt:          &now,
			FindingsBySeverity: map[string]int{},
			CreatedAt:          now,
		}
		if args.CreatedBy != "" {
			cb := args.CreatedBy
			run.CreatedBy = &cb
		}
		if args.ApplicationID != "" {
			id, err := uuid.Parse(args.ApplicationID)
			if err != nil {
				return Result{}, fmt.Errorf("assess: parse application_id: %w", err)
			}
			run.ScopeApplicationID = &id
		}
		if err := deps.RunsRepo.Insert(ctx.Ctx, run); err != nil {
			return Result{}, fmt.Errorf("assess: insert run: %w", err)
		}

		result, runErr := runAssessment(ctx.Ctx, deps, args, run.ID)

		// Finalise — completed on success, failed on error.
		end := time.Now().UTC()
		run.CompletedAt = &end
		run.FindingsTotal = result.FindingsCreated + result.FindingsReused
		run.FindingsCreatedCount = result.FindingsCreated
		run.FindingsReusedCount = result.FindingsReused
		if runErr != nil {
			run.Status = policy_assessment_runs.StatusFailed
			msg := runErr.Error()
			run.ErrorMessage = &msg
		} else {
			run.Status = policy_assessment_runs.StatusCompleted
		}
		if updErr := deps.RunsRepo.Update(ctx.Ctx, run); updErr != nil {
			// Persist-failure to the run row is worth surfacing but the
			// action body's error takes precedence.
			ctx.Log.Error("assess: finalize run failed", "err", updErr)
		}
		result.AssessmentRunID = run.ID.String()
		return result, runErr
	}
}

// runAssessment walks the account population once and dispatches
// every applicable policy. Counters live on Result so the deferred
// finalize-run path can read them whether the walk errored or not.
func runAssessment(ctx context.Context, deps Deps, args Args, runID uuid.UUID) (Result, error) {
	res := Result{}

	listFilter := accounts.ListFilter{ActiveOnly: false}
	if args.ApplicationID != "" {
		id, err := uuid.Parse(args.ApplicationID)
		if err != nil {
			return res, fmt.Errorf("parse application_id: %w", err)
		}
		listFilter.ApplicationID = &id
	}
	accs, _, err := deps.AccountsRepo.List(ctx, deps.DB, listFilter)
	if err != nil {
		return res, fmt.Errorf("list accounts: %w", err)
	}
	res.AccountsEvaluated = len(accs)

	mechanismAllow := allowList(args.Mechanisms)

	for _, acc := range accs {
		facts := factsForAccount(acc)
		facets := facetsForAccount(acc)
		entries := deps.Store.SelectByFacets(facets)

		for _, entry := range entries {
			if mechanismAllow != nil {
				if _, ok := mechanismAllow[entry.Manifest.Mechanism]; !ok {
					continue
				}
			}
			if !deps.Dispatcher.Has(entry.Manifest.Mechanism) {
				continue
			}
			res.PoliciesApplied++

			out, err := deps.Dispatcher.EvaluateEntry(ctx, entry, facts)
			if err != nil {
				return res, fmt.Errorf("dispatch %s/%s: %w", entry.CartridgeRef, entry.Manifest.RuleID, err)
			}
			if !out.Matched || out.Result.Decision == nil {
				// Generative ProjectedFacts are not persisted by this
				// action yet — they need a separate target store.
				continue
			}
			res.Matched++

			f := findingFromDecision(runID, entry, acc, out.Result.Decision)
			if insErr := deps.FindingsRepo.Insert(ctx, f); insErr != nil {
				if isDuplicateKey(insErr) {
					res.FindingsReused++
					continue
				}
				return res, fmt.Errorf("insert finding for %s/%s on account %s: %w",
					entry.CartridgeRef, entry.Manifest.RuleID, acc.ID, insErr)
			}
			res.FindingsCreated++
		}
	}
	return res, nil
}

// factsForAccount builds the engine Facts envelope from one account
// row. Target.Kind is "account" so policies can dispatch by shape.
func factsForAccount(acc *accounts.Account) policy_assessment.Facts {
	privileged := acc.IsPrivileged
	target := &policy_assessment.TargetFacts{
		Kind:                "account",
		ID:                  acc.ID.String(),
		Resource:            acc.Username,
		ResourceType:        "account",
		AccountIsPrivileged: &privileged,
	}
	if acc.Status != nil {
		target.AccountStatus = *acc.Status
	}
	resource := &policy_assessment.Resource{
		Type:       "Account",
		ID:         acc.ID.String(),
		Properties: map[string]any{},
	}
	resource.Properties["username"] = acc.Username
	resource.Properties["application_id"] = acc.ApplicationID.String()
	resource.Properties["is_active"] = acc.IsActive
	resource.Properties["is_privileged"] = acc.IsPrivileged
	resource.Properties["mfa_enabled"] = acc.MFAEnabled
	return policy_assessment.Facts{
		Target:   target,
		Resource: resource,
		Now:      time.Now().UTC(),
	}
}

// facetsForAccount is the coarse pre-filter set the assess action
// emits per account. Policies whose tags are a subset of this set
// get dispatched.
func facetsForAccount(acc *accounts.Account) []string {
	out := []string{
		"assessment",
		"scope:account",
		"application:" + acc.ApplicationID.String(),
		"resource:Account",
	}
	if acc.IsPrivileged {
		out = append(out, "account:privileged")
	}
	if !acc.IsActive {
		out = append(out, "account:inactive")
	}
	return out
}

// findingFromDecision maps one Decision into a Finding row.
//
// Kind: first string entry in Signals (the convention every kernel
// rule follows), falling back to the policy's rule_id basename.
// Severity: prefer the policy manifest's static severity, then the
// dynamic Decision.RiskLevel.
// evidence_hash is a stable digest over the identity tuple so a
// re-run that surfaces the same finding hits the DB unique
// constraint and is reused rather than duplicated.
func findingFromDecision(
	runID uuid.UUID,
	entry policy_assessment.Entry,
	acc *accounts.Account,
	dec *policy_assessment.Decision,
) *findings.Finding {
	now := time.Now().UTC()
	policyID := uuid.Nil
	if entry.Manifest.RuleID != "" {
		// Store column is FK on policies.id; we don't have the policy
		// row UUID here without an extra lookup, so leave it nil. The
		// rule_id is preserved via reasons + evidence_hash.
		policyID = uuid.Nil
	}
	severity := strings.ToLower(entry.Manifest.Severity)
	if severity == "" {
		severity = dec.RiskLevel
	}
	if severity == "" {
		severity = findings.SeverityLow
	}
	kind := firstStringSignal(dec.Signals)
	if kind == "" {
		kind = entry.Manifest.RuleID
	}
	if kind == "" {
		kind = "anomaly"
	}
	if len(kind) > 64 {
		kind = kind[:64]
	}
	accID := acc.ID
	f := &findings.Finding{
		ID:                        uuid.New(),
		AssessmentRunID:           runID,
		Kind:                      kind,
		AccountID:                 &accID,
		Severity:                  severity,
		Status:                    findings.StatusOpen,
		MatchedCapabilityGrantIDs: []string{},
		MatchedEffectiveGrantIDs:  []string{},
		MatchedAccessFactIDs:      []string{},
		EvidenceHash:              evidenceHash(entry, acc, dec),
		DetectedAt:                now,
		EvaluatedAt:               now,
	}
	if policyID != uuid.Nil {
		f.PolicyID = &policyID
	}
	return f
}

func firstStringSignal(signals []any) string {
	for _, s := range signals {
		if str, ok := s.(string); ok && str != "" {
			return str
		}
	}
	return ""
}

// evidenceHash is a stable digest over (cartridge, rule_id, account,
// kind-signal). Same tuple → same hash → DB unique constraint hits.
func evidenceHash(entry policy_assessment.Entry, acc *accounts.Account, dec *policy_assessment.Decision) string {
	h := sha256.New()
	h.Write([]byte(entry.CartridgeRef))
	h.Write([]byte{0})
	h.Write([]byte(entry.Manifest.RuleID))
	h.Write([]byte{0})
	h.Write([]byte(acc.ID.String()))
	h.Write([]byte{0})
	h.Write([]byte(firstStringSignal(dec.Signals)))
	return hex.EncodeToString(h.Sum(nil))
}

func allowList(mechs []string) map[string]struct{} {
	if len(mechs) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(mechs))
	for _, m := range mechs {
		out[m] = struct{}{}
	}
	return out
}

// isDuplicateKey checks both `database/sql` semantics and the bun
// driver's surface for the Postgres unique-violation. We rely on
// substring match against the constraint name as a defensive last
// resort.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "uq_findings_evidence") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "SQLSTATE 23505")
}

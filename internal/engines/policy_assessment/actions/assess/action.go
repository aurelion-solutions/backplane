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
	"log/slog"
	"strings"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/engines/owner_assignment"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	"github.com/aurelion-solutions/backplane/internal/engines/risk"
	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/aurelion-solutions/backplane/internal/inventory/consent"
	"github.com/aurelion-solutions/backplane/internal/inventory/evidence_chain"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_assessment_runs"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
	"github.com/aurelion-solutions/backplane/internal/inventory/principals"
	"github.com/aurelion-solutions/backplane/internal/inventory/secrets"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/aurelion-solutions/backplane/internal/inventory/workloads"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Deps is the composition-root injection set.
//
// AccountsRepo provides the population snapshot via List. Store +
// Dispatcher are the in-memory engine references that the worker
// composition root populates and keeps refreshed via the cartridge
// watcher. RunsRepo / FindingsRepo persist the action's output.
//
// Lineage + Snapshots + WorkloadsRepo + PrincipalsRepo power the
// workload assessment pass. Lineage and Snapshots are injected as ports
// so assess never imports workload_lineage.
type Deps struct {
	DB             *bun.DB
	AccountsRepo   accounts.Repository
	WorkloadsRepo  workloads.Repository
	PrincipalsRepo principals.Repository
	// SecretsPlain + SecretsCert power the secret-posture pass. Both
	// optional: when nil the pass is skipped (e.g. in narrow tests).
	SecretsPlain secrets.PlainRepository
	SecretsCert  secrets.CertRepository
	// ConsentApps + ConsentGrants power the consent-posture pass. Both
	// optional: when nil the pass is skipped.
	ConsentApps   consent.AppRepository
	ConsentGrants consent.GrantRepository
	// OwnerTerminus resolves a secret/consent owner's terminus for the
	// owner-terminated checks. Optional: nil leaves owner_terminus unset,
	// so that policy simply never matches.
	OwnerTerminus OwnerTerminusResolver
	Lineage       LineageResolver
	Snapshots     SnapshotWriter
	RunsRepo      policy_assessment_runs.Repository
	FindingsRepo  findings.Repository
	OutcomesSvc   *policy_evaluation_outcomes.Service
	EvidenceSvc   *evidence_chain.Service
	Store         *policy_assessment.Store
	Dispatcher    *policy_assessment.Dispatcher
	Log           *slog.Logger
	// OwnerResolver is an optional pre-built owner resolver. When nil,
	// the action loads it from DB via owner_assignment.Load. Inject in
	// tests to avoid requiring a real DB connection.
	OwnerResolver *owner_assignment.Resolver
}

// targetRef is the generalised per-target view the record helpers
// operate on. The account loop builds it from *accounts.Account; the
// workload loop builds it from (workload, principal).
//
// CRITICAL (F2): accountID must be set (non-nil) for the account path.
// evidenceHash and gapEvidenceHash hash over accountID for accounts,
// so the refactored helpers MUST preserve the same byte sequence.
type targetRef struct {
	principalID    *uuid.UUID
	accountID      *uuid.UUID // set for account path; nil for workload path
	applicationID  *uuid.UUID
	source         string
	isPrivileged   bool
	mfaEnabled     bool
	isActive       bool
	key            string // username for account; externalID for workload
	normalizedKind string // evidence_chain.NormalizedAccount or NormalizedWorkload
	normalizedID   uuid.UUID
	// targetType + targetID are the finding's artifact axis (one of the
	// findings.Target* constants). principalID is the identity axis and
	// is set independently — for an account it is the account's owner,
	// for a workload it is the workload's own principal.
	targetType string
	targetID   *uuid.UUID
}

// bindTarget writes both finding axes from the ref: the identity
// (principal_id, may be nil) and the discriminated artifact target.
func bindTarget(f *findings.Finding, ref targetRef) {
	f.PrincipalID = ref.principalID
	if ref.targetType != "" {
		tt := ref.targetType
		f.TargetType = &tt
		f.TargetID = ref.targetID
	}
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
		run.AsOf = &now
		if args.AsOf != "" {
			if t, perr := time.Parse(time.RFC3339, args.AsOf); perr == nil {
				asOf := t.UTC()
				run.AsOf = &asOf
			}
		}
		run.FindingsTotal = result.FindingsCreated + result.FindingsReused
		run.FindingsCreatedCount = result.FindingsCreated
		run.FindingsReusedCount = result.FindingsReused
		run.OutcomesByKind = map[string]int{
			policy_evaluation_outcomes.OutcomeMatched:      result.Matched,
			policy_evaluation_outcomes.OutcomeNotMatched:   result.NotMatched,
			policy_evaluation_outcomes.OutcomeNotEvaluable: result.NotEvaluable,
		}
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

// runAssessment walks the account population and (if wired) the
// workload population, dispatching applicable policies for each.
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

	// Owner routing: build the application→owner map once for the run.
	// Findings inherit their account's application owner.
	// When OwnerResolver is pre-injected (e.g. in tests), use it directly.
	resolver := deps.OwnerResolver
	if resolver == nil {
		var loadErr error
		resolver, loadErr = owner_assignment.Load(ctx, deps.DB)
		if loadErr != nil {
			return res, fmt.Errorf("load owner resolver: %w", loadErr)
		}
	}

	mechanismAllow := allowList(args.Mechanisms)
	cartridgeAllow := allowList(args.Cartridges)

	for _, acc := range accs {
		facts := factsForAccount(acc)
		facets := facetsForAccount(acc)
		entries := deps.Store.SelectByFacets(facets)

		for _, entry := range entries {
			if cartridgeAllow != nil {
				if _, ok := cartridgeAllow[entry.CartridgeRef]; !ok {
					continue
				}
			}
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

			ref := targetRefFromAccount(acc)
			switch {
			case out.NotEvaluable:
				if err := recordNotEvaluable(ctx, deps, &res, runID, entry, ref, resolver, out.MissingEvidence); err != nil {
					return res, err
				}
			case out.Matched && out.Result.Decision != nil:
				if err := recordMatched(ctx, deps, &res, runID, entry, ref, resolver, out.Result.Decision); err != nil {
					return res, err
				}
			default:
				if err := recordOutcome(ctx, deps, runID, entry, ref, policy_evaluation_outcomes.OutcomeNotMatched, nil); err != nil {
					return res, err
				}
				res.NotMatched++
			}
		}
	}

	// Workload pass: run after the account loop, before coverage checks.
	if deps.Lineage != nil && deps.WorkloadsRepo != nil {
		if err := runWorkloadPass(ctx, deps, args, &res, runID, resolver, mechanismAllow, cartridgeAllow); err != nil {
			return res, err
		}
	}

	// Secret pass: evaluate credential/certificate posture over both
	// secret entities. Skipped when the repos are not wired.
	if deps.SecretsPlain != nil && deps.SecretsCert != nil {
		if err := runSecretPass(ctx, deps, args, &res, runID, resolver, mechanismAllow, cartridgeAllow); err != nil {
			return res, err
		}
	}

	// Consent pass: evaluate delegated-access posture over presented
	// applications and the consent grants attached to them. Skipped when
	// the repos are not wired.
	if deps.ConsentApps != nil && deps.ConsentGrants != nil {
		if err := runConsentPass(ctx, deps, args, &res, runID, resolver, mechanismAllow, cartridgeAllow); err != nil {
			return res, err
		}
	}

	// Aggregate source/pipeline coverage gaps run once, after the
	// per-account loop, honouring the campaign's selected cartridges.
	if err := runCoverageChecks(ctx, deps, &res, runID, cartridgeAllow); err != nil {
		return res, err
	}
	return res, nil
}

// runWorkloadPass iterates the workload population and evaluates workload
// policies against each workload's resolved ownership chain.
func runWorkloadPass(
	ctx context.Context, deps Deps, args Args, res *Result, runID uuid.UUID,
	resolver *owner_assignment.Resolver, mechanismAllow, cartridgeAllow map[string]struct{},
) error {
	wls, _, err := deps.WorkloadsRepo.List(ctx, 0, 0)
	if err != nil {
		return fmt.Errorf("list workloads: %w", err)
	}
	// Filter by application if the campaign is scoped.
	if args.ApplicationID != "" {
		appID, _ := uuid.Parse(args.ApplicationID)
		filtered := wls[:0]
		for _, w := range wls {
			if w.ApplicationID != nil && *w.ApplicationID == appID {
				filtered = append(filtered, w)
			}
		}
		wls = filtered
	}
	res.WorkloadsEvaluated = len(wls)

	for _, w := range wls {
		chain, err := deps.Lineage.Resolve(ctx, w.ID)
		if err != nil {
			// Resolver errors are logged; the run continues.
			logWarn(deps.Log, "workload_lineage: resolve failed", w.ID, err)
			continue
		}

		// Persist the snapshot idempotently (append-only; DO NOTHING on
		// duplicate (workload_id, chain_hash)). This is the ONLY place
		// snapshots are written — never in the GET route (R1).
		if deps.Snapshots != nil {
			if snapErr := deps.Snapshots.RecordSnapshot(ctx, w.ID); snapErr != nil {
				logWarn(deps.Log, "workload_lineage: snapshot failed", w.ID, snapErr)
				// Non-fatal: assessment continues.
			}
		}

		// Resolve workload → principal. Skip if no principal row exists
		// (can't key a finding without a target; this is data-quality, not
		// a policy not_evaluable).
		if deps.PrincipalsRepo == nil {
			continue
		}
		principal, err := deps.PrincipalsRepo.GetByBody(ctx, shared.PrincipalKindWorkload, w.ID)
		if err != nil {
			if errors.Is(err, principals.ErrNotFound) {
				logWarn(deps.Log, "workload_lineage: no principal for workload", w.ID, nil)
				continue
			}
			return fmt.Errorf("get principal for workload %s: %w", w.ID, err)
		}

		facts := factsForWorkload(w, chain)
		facets := facetsForWorkload(w)
		entries := deps.Store.SelectByFacets(facets)

		for _, entry := range entries {
			if cartridgeAllow != nil {
				if _, ok := cartridgeAllow[entry.CartridgeRef]; !ok {
					continue
				}
			}
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
				return fmt.Errorf("dispatch workload %s/%s: %w", entry.CartridgeRef, entry.Manifest.RuleID, err)
			}

			ref := targetRefFromWorkload(w, principal)
			switch {
			case out.NotEvaluable:
				if err := recordNotEvaluable(ctx, deps, res, runID, entry, ref, resolver, out.MissingEvidence); err != nil {
					return err
				}
			case out.Matched && out.Result.Decision != nil:
				if err := recordMatched(ctx, deps, res, runID, entry, ref, resolver, out.Result.Decision); err != nil {
					return err
				}
			default:
				if err := recordOutcome(ctx, deps, runID, entry, ref, policy_evaluation_outcomes.OutcomeNotMatched, nil); err != nil {
					return err
				}
				res.NotMatched++
			}
		}
	}
	return nil
}

// runSecretPass evaluates credential/certificate posture policies over
// both secret entities. Each secret is a target in its own right
// (findings.TargetSecretPlain / TargetSecretCertificate); principal_id,
// when present, is the secret's owner on the identity axis.
func runSecretPass(
	ctx context.Context, deps Deps, args Args, res *Result, runID uuid.UUID,
	resolver *owner_assignment.Resolver, mechanismAllow, cartridgeAllow map[string]struct{},
) error {
	now := time.Now().UTC()

	var scopeApp *uuid.UUID
	if args.ApplicationID != "" {
		id, err := uuid.Parse(args.ApplicationID)
		if err != nil {
			return fmt.Errorf("parse application_id: %w", err)
		}
		scopeApp = &id
	}

	plains, _, err := deps.SecretsPlain.List(ctx, deps.DB, secrets.PlainListFilter{TargetApplicationID: scopeApp})
	if err != nil {
		return fmt.Errorf("list secret_plain: %w", err)
	}
	for _, s := range plains {
		facts := factsForSecretPlain(s, now, ownerTerminus(ctx, deps, s.PrincipalID))
		facets := facetsForSecret(findings.TargetSecretPlain, secretAppID(s.TargetApplicationID, s.FoundInApplicationID), s.IsPrivileged)
		ref := targetRefFromSecretPlain(s)
		if err := evaluateTarget(ctx, deps, res, runID, facts, facets, ref, resolver, mechanismAllow, cartridgeAllow); err != nil {
			return err
		}
		res.SecretsEvaluated++
	}

	certs, _, err := deps.SecretsCert.List(ctx, deps.DB, secrets.CertListFilter{TargetApplicationID: scopeApp})
	if err != nil {
		return fmt.Errorf("list secret_certificate: %w", err)
	}
	for _, c := range certs {
		facts := factsForSecretCertificate(c, now, ownerTerminus(ctx, deps, c.PrincipalID))
		facets := facetsForSecret(findings.TargetSecretCertificate, secretAppID(c.TargetApplicationID, c.FoundInApplicationID), c.IsPrivileged)
		ref := targetRefFromSecretCertificate(c)
		if err := evaluateTarget(ctx, deps, res, runID, facts, facets, ref, resolver, mechanismAllow, cartridgeAllow); err != nil {
			return err
		}
		res.SecretsEvaluated++
	}
	return nil
}

// runConsentPass evaluates delegated-access posture over presented
// applications and their consent grants. A consent grant is evidence of
// delegated access, not identity truth: app-level posture (display-name
// collision, publisher mismatch) targets the consented_application, while
// grant-level posture (sensitive/privileged scope, missing/terminated
// owner, staleness) targets the consent_grant and denormalises the
// owning app's origin/verification into the grant facts.
func runConsentPass(
	ctx context.Context, deps Deps, args Args, res *Result, runID uuid.UUID,
	resolver *owner_assignment.Resolver, mechanismAllow, cartridgeAllow map[string]struct{},
) error {
	now := time.Now().UTC()

	apps, _, err := deps.ConsentApps.List(ctx, deps.DB, consent.AppListFilter{})
	if err != nil {
		return fmt.Errorf("list consented_application: %w", err)
	}
	byID := make(map[uuid.UUID]*consent.ConsentedApplication, len(apps))
	for _, a := range apps {
		byID[a.ID] = a
		facts := factsForConsentedApp(a, now)
		ref := targetRefFromConsentedApp(a)
		if err := evaluateTarget(ctx, deps, res, runID, facts, facetsForConsentApp(), ref, resolver, mechanismAllow, cartridgeAllow); err != nil {
			return err
		}
		res.ConsentEvaluated++
	}

	grants, _, err := deps.ConsentGrants.List(ctx, deps.DB, consent.GrantListFilter{})
	if err != nil {
		return fmt.Errorf("list consent_grant: %w", err)
	}
	for _, g := range grants {
		app := byID[g.ConsentedApplicationID]
		if app == nil {
			// Orphan grant whose app fell outside the listing — skip
			// rather than evaluate against unknown app posture.
			continue
		}
		facts := factsForConsentGrant(g, app, now, ownerTerminus(ctx, deps, g.ConsentingPrincipalID))
		ref := targetRefFromConsentGrant(g, app)
		if err := evaluateTarget(ctx, deps, res, runID, facts, facetsForConsentGrant(), ref, resolver, mechanismAllow, cartridgeAllow); err != nil {
			return err
		}
		res.ConsentEvaluated++
	}
	return nil
}

// factsForConsentedApp builds the Facts envelope for one presented app.
// Only self-asserted claims and the resolver's verdict — no scopes (those
// live on the grant).
func factsForConsentedApp(a *consent.ConsentedApplication, now time.Time) policy_assessment.Facts {
	props := map[string]any{
		"kind":                  "consented_application",
		"origin":                a.Origin,
		"resolution_confidence": a.ResolutionConfidence,
		"verified_publisher":    a.VerifiedPublisher,
		"is_resolved":           a.ResolvedPrincipalID != nil,
		"has_publisher":         a.Publisher != nil,
	}
	if a.Publisher != nil {
		props["publisher"] = *a.Publisher
	}
	if a.DisplayName != nil {
		props["display_name"] = *a.DisplayName
	}
	label := a.ClientID
	if a.DisplayName != nil {
		label = *a.DisplayName
	}
	return policy_assessment.Facts{
		Target:   &policy_assessment.TargetFacts{Kind: "consented_application", ID: a.ID.String(), Resource: label, ResourceType: "consented_application"},
		Resource: &policy_assessment.Resource{Type: "ConsentedApplication", ID: a.ID.String(), Properties: props},
		Now:      now,
	}
}

// factsForConsentGrant builds the Facts envelope for one consent grant,
// denormalising the owning app's origin/verification so grant policies
// can reason about both the scope and who it was granted to.
func factsForConsentGrant(g *consent.ConsentGrant, app *consent.ConsentedApplication, now time.Time, terminus string) policy_assessment.Facts {
	props := map[string]any{
		"kind":                      "consent_grant",
		"grant_type":                g.GrantType,
		"scopes":                    g.Scopes,
		"scope_count":               len(g.Scopes),
		"has_owner":                 g.ConsentingPrincipalID != nil,
		"is_active":                 g.IsActive,
		"app_origin":                app.Origin,
		"app_verified_publisher":    app.VerifiedPublisher,
		"app_resolution_confidence": app.ResolutionConfidence,
	}
	if terminus != "" {
		props["owner_terminus"] = terminus
	}
	if g.LastUsedAt != nil {
		props["days_since_last_use"] = daysBetween(*g.LastUsedAt, now)
	}
	return policy_assessment.Facts{
		Target:   &policy_assessment.TargetFacts{Kind: "consent_grant", ID: g.ID.String(), Resource: g.ExternalID, ResourceType: "consent_grant"},
		Resource: &policy_assessment.Resource{Type: "ConsentGrant", ID: g.ID.String(), Properties: props},
		Now:      now,
		EvidencePresent: map[string]bool{
			"last_used_evidence": g.LastUsedAt != nil,
		},
	}
}

// facetsForConsentApp / facetsForConsentGrant carry scope:consent (never
// scope:account / scope:workload / scope:secret) so only consent policies
// can satisfy tags ⊆ facets, and split application vs grant policies.
func facetsForConsentApp() []string {
	return []string{"assessment", "scope:consent", "consent:application"}
}

func facetsForConsentGrant() []string {
	return []string{"assessment", "scope:consent", "consent:grant"}
}

// targetRefFromConsentedApp builds a targetRef from a presented app. The
// identity axis is the resolved principal when the resolver linked one.
func targetRefFromConsentedApp(a *consent.ConsentedApplication) targetRef {
	id := a.ID
	label := a.ClientID
	if a.DisplayName != nil {
		label = *a.DisplayName
	}
	return targetRef{
		principalID:    a.ResolvedPrincipalID,
		source:         a.Source,
		isActive:       a.IsActive,
		key:            label,
		normalizedKind: evidence_chain.NormalizedConsentedApplication,
		normalizedID:   a.ID,
		targetType:     findings.TargetConsentedApplication,
		targetID:       &id,
	}
}

// targetRefFromConsentGrant builds a targetRef from a grant. The identity
// axis is the consenting principal (the subject who granted access).
func targetRefFromConsentGrant(g *consent.ConsentGrant, _ *consent.ConsentedApplication) targetRef {
	id := g.ID
	return targetRef{
		principalID:    g.ConsentingPrincipalID,
		source:         g.Source,
		isActive:       g.IsActive,
		key:            g.ExternalID,
		normalizedKind: evidence_chain.NormalizedConsentGrant,
		normalizedID:   g.ID,
		targetType:     findings.TargetConsentGrant,
		targetID:       &id,
	}
}

// evaluateTarget runs every policy whose tags are a subset of facets
// against facts, recording the matched / not_evaluable / not_matched
// outcome plus finding for each. Shared by the secret pass; the account
// and workload loops inline the same shape for historical-hash reasons.
func evaluateTarget(
	ctx context.Context, deps Deps, res *Result, runID uuid.UUID,
	facts policy_assessment.Facts, facets []string, ref targetRef,
	resolver *owner_assignment.Resolver, mechanismAllow, cartridgeAllow map[string]struct{},
) error {
	for _, entry := range deps.Store.SelectByFacets(facets) {
		if cartridgeAllow != nil {
			if _, ok := cartridgeAllow[entry.CartridgeRef]; !ok {
				continue
			}
		}
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
			return fmt.Errorf("dispatch %s/%s: %w", entry.CartridgeRef, entry.Manifest.RuleID, err)
		}
		switch {
		case out.NotEvaluable:
			if err := recordNotEvaluable(ctx, deps, res, runID, entry, ref, resolver, out.MissingEvidence); err != nil {
				return err
			}
		case out.Matched && out.Result.Decision != nil:
			if err := recordMatched(ctx, deps, res, runID, entry, ref, resolver, out.Result.Decision); err != nil {
				return err
			}
		default:
			if err := recordOutcome(ctx, deps, runID, entry, ref, policy_evaluation_outcomes.OutcomeNotMatched, nil); err != nil {
				return err
			}
			res.NotMatched++
		}
	}
	return nil
}

// daysBetween returns whole days from a to b (negative if b precedes a).
func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24)
}

// secretAppID picks the application a secret is routed to for owner
// resolution: its target application if known, else where it was found.
func secretAppID(target, foundIn *uuid.UUID) *uuid.UUID {
	if target != nil {
		return target
	}
	return foundIn
}

// ownerTerminus resolves the terminus of a secret's owning principal,
// or "" when there is no owner / no resolver / it cannot be resolved.
// Resolution errors are swallowed to "" so a single bad lookup never
// fails the whole run.
func ownerTerminus(ctx context.Context, deps Deps, principalID *uuid.UUID) string {
	if principalID == nil || deps.OwnerTerminus == nil {
		return ""
	}
	terminus, err := deps.OwnerTerminus.Resolve(ctx, *principalID)
	if err != nil {
		return ""
	}
	return terminus
}

// factsForSecretPlain builds the Facts envelope for one plain secret.
// Time math is precomputed here; policies only compare the numbers.
func factsForSecretPlain(s *secrets.SecretPlain, now time.Time, terminus string) policy_assessment.Facts {
	props := map[string]any{
		"kind":          "secret_plain",
		"type":          s.Type,
		"is_privileged": s.IsPrivileged,
		"is_active":     s.IsActive,
		"has_owner":     s.PrincipalID != nil,
		"has_expiry":    s.ExpiresAt != nil,
		"scope_count":   len(s.Scopes),
	}
	if terminus != "" {
		props["owner_terminus"] = terminus
	}
	if s.ExpiresAt != nil {
		props["days_until_expiry"] = daysBetween(now, *s.ExpiresAt)
	}
	if s.IssuedAt != nil {
		props["age_days"] = daysBetween(*s.IssuedAt, now)
	}
	if s.LastUsedAt != nil {
		props["days_since_last_use"] = daysBetween(*s.LastUsedAt, now)
	}
	return policy_assessment.Facts{
		Target:   &policy_assessment.TargetFacts{Kind: "secret_plain", ID: s.ID.String(), Resource: s.Label, ResourceType: "secret_plain"},
		Resource: &policy_assessment.Resource{Type: "SecretPlain", ID: s.ID.String(), Properties: props},
		Now:      now,
		EvidencePresent: map[string]bool{
			"last_used_evidence": s.LastUsedAt != nil,
			"expiry_evidence":    s.ExpiresAt != nil,
		},
	}
}

// factsForSecretCertificate builds the Facts envelope for one certificate.
func factsForSecretCertificate(c *secrets.SecretCertificate, now time.Time, terminus string) policy_assessment.Facts {
	props := map[string]any{
		"kind":          "secret_certificate",
		"format":        c.Format,
		"usage":         c.Usage,
		"is_privileged": c.IsPrivileged,
		"is_active":     c.IsActive,
		"has_owner":     c.PrincipalID != nil,
		"has_expiry":    c.NotAfter != nil,
		"self_signed":   c.SelfSigned,
		"is_ca":         c.IsCA,
	}
	if terminus != "" {
		props["owner_terminus"] = terminus
	}
	if c.KeyAlgorithm != nil {
		props["key_algorithm"] = *c.KeyAlgorithm
	}
	if c.KeySize != nil {
		props["key_size"] = *c.KeySize
	}
	if c.NotAfter != nil {
		props["days_until_expiry"] = daysBetween(now, *c.NotAfter)
	}
	if c.NotBefore != nil && c.NotAfter != nil {
		props["validity_days"] = daysBetween(*c.NotBefore, *c.NotAfter)
	}
	if c.LastUsedAt != nil {
		props["days_since_last_use"] = daysBetween(*c.LastUsedAt, now)
	}
	return policy_assessment.Facts{
		Target:   &policy_assessment.TargetFacts{Kind: "secret_certificate", ID: c.ID.String(), Resource: c.Label, ResourceType: "secret_certificate"},
		Resource: &policy_assessment.Resource{Type: "SecretCertificate", ID: c.ID.String(), Properties: props},
		Now:      now,
		EvidencePresent: map[string]bool{
			"last_used_evidence": c.LastUsedAt != nil,
			"expiry_evidence":    c.NotAfter != nil,
		},
	}
}

// facetsForSecret is the coarse pre-filter set for secret policy
// selection. It carries scope:secret (never scope:account/scope:workload)
// so only secret policies can satisfy tags ⊆ facets.
func facetsForSecret(targetType string, appID *uuid.UUID, isPrivileged bool) []string {
	out := []string{"assessment", "scope:secret"}
	switch targetType {
	case findings.TargetSecretPlain:
		out = append(out, "secret:plain")
	case findings.TargetSecretCertificate:
		out = append(out, "secret:certificate")
	}
	if appID != nil {
		out = append(out, "application:"+appID.String())
	}
	if isPrivileged {
		out = append(out, "secret:privileged")
	}
	return out
}

// targetRefFromSecretPlain builds a targetRef from a plain secret.
func targetRefFromSecretPlain(s *secrets.SecretPlain) targetRef {
	id := s.ID
	return targetRef{
		principalID:    s.PrincipalID,
		applicationID:  secretAppID(s.TargetApplicationID, s.FoundInApplicationID),
		source:         s.Source,
		isPrivileged:   s.IsPrivileged,
		isActive:       s.IsActive,
		key:            s.Label,
		normalizedKind: evidence_chain.NormalizedSecretPlain,
		normalizedID:   s.ID,
		targetType:     findings.TargetSecretPlain,
		targetID:       &id,
	}
}

// targetRefFromSecretCertificate builds a targetRef from a certificate.
func targetRefFromSecretCertificate(c *secrets.SecretCertificate) targetRef {
	id := c.ID
	return targetRef{
		principalID:    c.PrincipalID,
		applicationID:  secretAppID(c.TargetApplicationID, c.FoundInApplicationID),
		source:         c.Source,
		isPrivileged:   c.IsPrivileged,
		isActive:       c.IsActive,
		key:            c.Label,
		normalizedKind: evidence_chain.NormalizedSecretCertificate,
		normalizedID:   c.ID,
		targetType:     findings.TargetSecretCertificate,
		targetID:       &id,
	}
}

// targetRefFromAccount builds a targetRef from an account row.
//
// CRITICAL (F2): accountID is set here. The evidence hash helpers feed
// accountID into their digest for the account path — byte-identical to
// the pre-refactor implementation.
func targetRefFromAccount(acc *accounts.Account) targetRef {
	accID := acc.ID
	var appID *uuid.UUID
	if acc.ApplicationID != uuid.Nil {
		id := acc.ApplicationID
		appID = &id
	}
	kind := evidence_chain.NormalizedAccount
	return targetRef{
		accountID:      &accID,
		principalID:    acc.PrincipalID, // identity axis: the account's owner (may be nil)
		applicationID:  appID,
		source:         acc.Source,
		isPrivileged:   acc.IsPrivileged,
		mfaEnabled:     acc.MFAEnabled,
		isActive:       acc.IsActive,
		key:            acc.Username,
		normalizedKind: kind,
		normalizedID:   acc.ID,
		targetType:     findings.TargetAccount,
		targetID:       &accID,
	}
}

// targetRefFromWorkload builds a targetRef from a workload + principal.
func targetRefFromWorkload(w *workloads.Workload, p *principals.Principal) targetRef {
	pid := p.ID
	wid := w.ID
	ref := targetRef{
		principalID:    &pid,
		key:            w.ExternalID,
		normalizedKind: evidence_chain.NormalizedWorkload,
		normalizedID:   w.ID,
		targetType:     findings.TargetWorkload,
		targetID:       &wid,
	}
	if w.ApplicationID != nil {
		id := *w.ApplicationID
		ref.applicationID = &id
	}
	return ref
}

// recordMatched persists the matched outcome, the violation finding,
// and the evidence chain linking them.
func recordMatched(
	ctx context.Context, deps Deps, res *Result, runID uuid.UUID,
	entry policy_assessment.Entry, ref targetRef, resolver *owner_assignment.Resolver,
	dec *policy_assessment.Decision,
) error {
	res.Matched++
	outcome, err := recordOutcomeRow(ctx, deps, runID, entry, ref, policy_evaluation_outcomes.OutcomeMatched, nil)
	if err != nil {
		return err
	}

	f := findingFromDecision(runID, entry, ref, dec)
	stampTriage(f, ref, entry, resolver)
	var findingID *uuid.UUID
	if insErr := deps.FindingsRepo.Insert(ctx, f); insErr != nil {
		if !isDuplicateKey(insErr) {
			return fmt.Errorf("insert finding for %s/%s on target %s: %w",
				entry.CartridgeRef, entry.Manifest.RuleID, ref.key, insErr)
		}
		// Re-confirm the existing finding for this run so current-posture
		// views still see it after a re-run.
		if _, touchErr := deps.FindingsRepo.TouchLastSeen(ctx, f.EvidenceHash, runID, f.EvaluatedAt); touchErr != nil {
			return fmt.Errorf("touch finding for %s/%s on target %s: %w",
				entry.CartridgeRef, entry.Manifest.RuleID, ref.key, touchErr)
		}
		res.FindingsReused++
	} else {
		res.FindingsCreated++
		fid := f.ID
		findingID = &fid
	}
	return recordChain(ctx, deps, res, runID, entry, ref, findingID, outcomeID(outcome))
}

// recordNotEvaluable persists the not_evaluable outcome, a derived
// evidence_gap finding (the Blind Spot), and the evidence chain.
func recordNotEvaluable(
	ctx context.Context, deps Deps, res *Result, runID uuid.UUID,
	entry policy_assessment.Entry, ref targetRef, resolver *owner_assignment.Resolver,
	missing []string,
) error {
	res.NotEvaluable++
	outcome, err := recordOutcomeRow(ctx, deps, runID, entry, ref, policy_evaluation_outcomes.OutcomeNotEvaluable, missing)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	f := &findings.Finding{
		ID:                        uuid.New(),
		AssessmentRunID:           runID,
		LastSeenRunID:             runID,
		Kind:                      findings.KindEvidenceGap,
		Severity:                  findings.SeverityMedium,
		Status:                    findings.StatusOpen,
		MatchedCapabilityGrantIDs: []string{},
		MatchedEffectiveGrantIDs:  []string{},
		MatchedAccessFactIDs:      []string{},
		EvidenceHash:              gapEvidenceHash(entry, ref),
		DetectedAt:                now,
		EvaluatedAt:               now,
	}
	bindTarget(f, ref)
	stampTriage(f, ref, entry, resolver)
	var findingID *uuid.UUID
	if insErr := deps.FindingsRepo.Insert(ctx, f); insErr != nil {
		if !isDuplicateKey(insErr) {
			return fmt.Errorf("insert evidence_gap for %s/%s on target %s: %w",
				entry.CartridgeRef, entry.Manifest.RuleID, ref.key, insErr)
		}
		// Re-confirm the existing evidence_gap for this run.
		if _, touchErr := deps.FindingsRepo.TouchLastSeen(ctx, f.EvidenceHash, runID, f.EvaluatedAt); touchErr != nil {
			return fmt.Errorf("touch evidence_gap for %s/%s on target %s: %w",
				entry.CartridgeRef, entry.Manifest.RuleID, ref.key, touchErr)
		}
		res.FindingsReused++
	} else {
		res.FindingsCreated++
		res.EvidenceGaps++
		fid := f.ID
		findingID = &fid
	}
	return recordChain(ctx, deps, res, runID, entry, ref, findingID, outcomeID(outcome))
}

// recordOutcome records a PEO row and discards it (used for not_matched
// where nothing downstream needs the id).
func recordOutcome(
	ctx context.Context, deps Deps, runID uuid.UUID,
	entry policy_assessment.Entry, ref targetRef, outcome string, missing []string,
) error {
	_, err := recordOutcomeRow(ctx, deps, runID, entry, ref, outcome, missing)
	return err
}

func recordOutcomeRow(
	ctx context.Context, deps Deps, runID uuid.UUID,
	entry policy_assessment.Entry, ref targetRef, outcome string, missing []string,
) (*policy_evaluation_outcomes.PolicyEvaluationOutcome, error) {
	if deps.OutcomesSvc == nil {
		return nil, nil
	}
	params := policy_evaluation_outcomes.RecordParams{
		AssessmentRunID: runID,
		CartridgeID:     entry.CartridgeRef,
		RuleID:          entry.Manifest.RuleID,
		TargetKey:       ref.key,
		Outcome:         outcome,
		MissingEvidence: missing,
	}
	if ref.accountID != nil {
		params.TargetType = policy_evaluation_outcomes.TargetAccount
		params.TargetRef = ref.accountID
	} else {
		params.TargetType = policy_evaluation_outcomes.TargetWorkload
		params.TargetRef = ref.principalID
	}
	row, err := deps.OutcomesSvc.RecordOutcome(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("record outcome %s for %s/%s: %w", outcome, entry.CartridgeRef, entry.Manifest.RuleID, err)
	}
	return row, nil
}

func recordChain(
	ctx context.Context, deps Deps, res *Result, runID uuid.UUID,
	entry policy_assessment.Entry, ref targetRef, findingID, outcomeID *uuid.UUID,
) error {
	if deps.EvidenceSvc == nil {
		return nil
	}
	kind := ref.normalizedKind
	normID := ref.normalizedID
	if _, err := deps.EvidenceSvc.RecordChain(ctx, evidence_chain.RecordParams{
		ScanRunID:      runID,
		FindingID:      findingID,
		OutcomeID:      outcomeID,
		NormalizedKind: &kind,
		NormalizedID:   &normID,
		PolicyRef:      entry.CartridgeRef + "/" + entry.Manifest.RuleID,
	}); err != nil {
		return fmt.Errorf("record evidence chain for %s/%s: %w", entry.CartridgeRef, entry.Manifest.RuleID, err)
	}
	res.ChainsRecorded++
	return nil
}

func outcomeID(o *policy_evaluation_outcomes.PolicyEvaluationOutcome) *uuid.UUID {
	if o == nil {
		return nil
	}
	id := o.ID
	return &id
}

// gapEvidenceHash is a stable digest for an evidence_gap finding.
//
// CRITICAL (F2): for the account path (ref.accountID != nil), this
// hashes over the account ID — byte-identical to the pre-refactor
// implementation. The workload path hashes over principalID + key, a
// distinct additive input space.
func gapEvidenceHash(entry policy_assessment.Entry, ref targetRef) string {
	h := sha256.New()
	h.Write([]byte(entry.CartridgeRef))
	h.Write([]byte{0})
	h.Write([]byte(entry.Manifest.RuleID))
	h.Write([]byte{0})
	if ref.accountID != nil {
		// Account path: hash over account ID — preserved byte-for-byte.
		h.Write([]byte(ref.accountID.String()))
	} else if ref.principalID != nil {
		h.Write([]byte(ref.principalID.String()))
	}
	h.Write([]byte{0})
	h.Write([]byte("evidence_gap"))
	return hex.EncodeToString(h.Sum(nil))
}

// factsForAccount builds the engine Facts envelope from one account row.
func factsForAccount(acc *accounts.Account) policy_assessment.Facts {
	privileged := acc.IsPrivileged
	mfaEnabled := acc.MFAEnabled
	target := &policy_assessment.TargetFacts{
		Kind:                "account",
		ID:                  acc.ID.String(),
		Resource:            acc.Username,
		ResourceType:        "account",
		AccountIsPrivileged: &privileged,
		AccountMFAEnabled:   &mfaEnabled,
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
		EvidencePresent: map[string]bool{
			"mfa_evidence":       acc.MFAEvidenceAt != nil,
			"owner_evidence":     acc.OwnerEvidenceAt != nil,
			"last_used_evidence": acc.LastUsedEvidenceAt != nil,
			"subject_linkage":    false,
			"activity_telemetry": false,
			"initiative_state":   false,
		},
		// Subject is nil for the account path (omitempty marshals it away).
	}
}

// facetsForAccount is the coarse pre-filter set the assess action emits
// per account. Policies whose tags are a subset of this set are dispatched.
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

// factsForWorkload builds the engine Facts envelope for one workload and
// its resolved ownership chain.
func factsForWorkload(w *workloads.Workload, chain ResolvedChain) policy_assessment.Facts {
	ownershipResolved := chain.Terminus == TerminusActiveHuman || chain.Terminus == TerminusTerminatedHuman
	target := &policy_assessment.TargetFacts{
		Kind:         "workload",
		ID:           w.ID.String(),
		Resource:     w.ExternalID,
		ResourceType: "workload",
	}
	ownership := &policy_assessment.OwnershipFacts{
		Terminus:            chain.Terminus,
		OwnerPersonID:       chain.OwnerPersonID,
		OwnerLabel:          chain.OwnerLabel,
		LastTerminationDate: chain.LastTerminationDate,
	}
	subject := &policy_assessment.SubjectFacts{
		Kind:      "workload",
		ID:        w.ID.String(),
		Ownership: ownership,
	}
	return policy_assessment.Facts{
		Target:  target,
		Subject: subject,
		Now:     time.Now().UTC(),
		// subject_linkage and ownership_resolved are true only when the
		// chain resolved to a human terminus. When unowned or broken,
		// both are false → stack_check.requires fires → not_evaluable
		// (Blind Spot), not a silent not_matched (honest blind spot).
		EvidencePresent: map[string]bool{
			"subject_linkage":    ownershipResolved,
			"ownership_resolved": ownershipResolved,
		},
	}
}

// TerminusActiveHuman / TerminusTerminatedHuman are the two human termini.
// Defined here so factsForWorkload does not need to import workload_lineage.
const (
	TerminusActiveHuman     = "active_human"
	TerminusTerminatedHuman = "terminated_human"
)

// facetsForWorkload builds the facet set for workload policy selection.
//
// MANDATORY (F1): this set MUST NOT contain "scope:account" or
// "resource:Account". Account policies carry "scope:account" in their
// required tag set; since this facet set does not contain "scope:account",
// no account policy can satisfy (policy.tags ⊆ facetsForWorkload), preventing
// cross-evaluation of the two distinct populations.
func facetsForWorkload(w *workloads.Workload) []string {
	out := []string{
		"assessment",
		"scope:workload",
		"subject:workload",
	}
	if w.ApplicationID != nil {
		out = append(out, "application:"+w.ApplicationID.String())
	}
	return out
}

// findingFromDecision maps one Decision into a Finding row.
func findingFromDecision(
	runID uuid.UUID,
	entry policy_assessment.Entry,
	ref targetRef,
	dec *policy_assessment.Decision,
) *findings.Finding {
	now := time.Now().UTC()
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
	f := &findings.Finding{
		ID:                        uuid.New(),
		AssessmentRunID:           runID,
		LastSeenRunID:             runID,
		Kind:                      kind,
		Severity:                  severity,
		Status:                    findings.StatusOpen,
		MatchedCapabilityGrantIDs: []string{},
		MatchedEffectiveGrantIDs:  []string{},
		MatchedAccessFactIDs:      []string{},
		EvidenceHash:              evidenceHash(entry, ref, dec),
		DetectedAt:                now,
		EvaluatedAt:               now,
	}
	bindTarget(f, ref)
	return f
}

// stampTriage denormalises the triage attributes onto a finding.
func stampTriage(f *findings.Finding, ref targetRef, entry policy_assessment.Entry, resolver *owner_assignment.Resolver) {
	f.ApplicationID = ref.applicationID
	if ref.source != "" {
		s := ref.source
		f.Source = &s
	}
	cr := entry.CartridgeRef
	f.CartridgeRef = &cr
	if ref.applicationID != nil {
		if owner := resolver.ForApplication(*ref.applicationID); owner != "" {
			f.OwnerRef = &owner
		}
	}
	if rec := entry.Manifest.DefaultRecommendation; rec != "" {
		f.RecommendedAction = &rec
	}
	if fm := entry.Manifest.Finding; fm != nil && fm.Remediation != "" {
		rem := fm.Remediation
		f.Remediation = &rem
	}
	scored := risk.Score(risk.Input{
		Severity:   f.Severity,
		Kind:       f.Kind,
		Privileged: ref.isPrivileged,
		MFAEnabled: ref.mfaEnabled,
		Active:     ref.isActive,
	})
	f.PriorityScore = scored.Score
	f.PriorityFactors = toFindingFactors(scored.Factors)
}

func toFindingFactors(in []risk.Factor) []findings.PriorityFactor {
	out := make([]findings.PriorityFactor, len(in))
	for i, x := range in {
		out[i] = findings.PriorityFactor{Name: x.Name, Points: x.Points}
	}
	return out
}

func firstStringSignal(signals []any) string {
	for _, s := range signals {
		if str, ok := s.(string); ok && str != "" {
			return str
		}
	}
	return ""
}

// evidenceHash is a stable digest over (cartridge, rule_id, target, signal).
//
// CRITICAL (F2): for the account path (ref.accountID != nil), this hashes
// over the account ID — byte-identical to the pre-refactor implementation.
// The workload path hashes over principalID + key, a distinct additive input
// space that does not collide with account hashes.
func evidenceHash(entry policy_assessment.Entry, ref targetRef, dec *policy_assessment.Decision) string {
	h := sha256.New()
	h.Write([]byte(entry.CartridgeRef))
	h.Write([]byte{0})
	h.Write([]byte(entry.Manifest.RuleID))
	h.Write([]byte{0})
	if ref.accountID != nil {
		// Account path: hash over account ID — preserved byte-for-byte.
		h.Write([]byte(ref.accountID.String()))
	} else if ref.principalID != nil {
		h.Write([]byte(ref.principalID.String()))
	}
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
// driver's surface for the Postgres unique-violation.
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

func logWarn(log *slog.Logger, msg string, workloadID uuid.UUID, err error) {
	if log == nil {
		return
	}
	if err != nil {
		log.Warn(msg, slog.String("workload_id", workloadID.String()), slog.Any("err", err))
	} else {
		log.Warn(msg, slog.String("workload_id", workloadID.String()))
	}
}

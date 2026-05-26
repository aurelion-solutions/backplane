// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package assess_test

import (
	"context"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/actions/assess"
	"github.com/aurelion-solutions/backplane/internal/inventory/consent"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// -------- consent fakes --------

type fakeConsentApps struct {
	apps []*consent.ConsentedApplication
}

func (f *fakeConsentApps) Upsert(_ context.Context, _ bun.IDB, _ *consent.ConsentedApplication) error {
	return nil
}
func (f *fakeConsentApps) List(_ context.Context, _ bun.IDB, _ consent.AppListFilter) ([]*consent.ConsentedApplication, int, error) {
	return f.apps, len(f.apps), nil
}

type fakeConsentGrants struct{ grants []*consent.ConsentGrant }

func (f *fakeConsentGrants) Upsert(_ context.Context, _ bun.IDB, _ *consent.ConsentGrant) error {
	return nil
}
func (f *fakeConsentGrants) List(_ context.Context, _ bun.IDB, _ consent.GrantListFilter) ([]*consent.ConsentGrant, int, error) {
	return f.grants, len(f.grants), nil
}

// fakeOwnerTerminus resolves a fixed terminus per principal id.
type fakeOwnerTerminus struct{ byPrincipal map[uuid.UUID]string }

func (f *fakeOwnerTerminus) Resolve(_ context.Context, principalID uuid.UUID) (string, error) {
	return f.byPrincipal[principalID], nil
}

const consentTerminatedRego = `
package ispm_consent_grants.grant.consent_owner_terminated
import future.keywords.if
default decision := null
decision := result if {
	input.resource.properties.owner_terminus == "terminated_human"
	result := {
		"risk_level": "high",
		"signals":    ["consent_owner_terminated"],
		"reasons":    [],
	}
}
`

// buildConsentPolicyStore loads a store+dispatcher for the consent
// owner-terminated grant policy, tagged for the consent grant population.
func buildConsentPolicyStore(t *testing.T) (*policy_assessment.Store, *policy_assessment.Dispatcher) {
	t.Helper()
	dir := t.TempDir()
	manifest := writePolicyFiles(t, dir,
		"consent_owner_terminated",
		"ispm_consent_grants.grant.consent_owner_terminated",
		consentTerminatedRego,
		[]string{"assessment", "scope:consent", "consent:grant"},
		nil,
	)
	return buildStoreAndDispatcher(t, "ispm-consent-grants", []cartridges.Manifest{manifest})
}

// TestConsentPass_TerminatedConsenter_FindingCreated verifies the consent
// pass evaluates a grant whose consenting principal is a terminated human
// and creates a finding keyed to the grant (target) and that principal
// (identity axis).
func TestConsentPass_TerminatedConsenter_FindingCreated(t *testing.T) {
	store, dispatcher := buildConsentPolicyStore(t)

	appID := uuid.New()
	grantID := uuid.New()
	consenterPID := uuid.New()

	app := &consent.ConsentedApplication{
		ID:                   appID,
		Source:               "test",
		ClientID:             "app-xyz",
		Origin:               consent.OriginThirdParty,
		ResolutionConfidence: consent.ConfidenceUnresolved,
	}
	grant := &consent.ConsentGrant{
		ID:                     grantID,
		Source:                 "test",
		ExternalID:             "grant-1",
		ConsentedApplicationID: appID,
		ConsentingPrincipalID:  &consenterPID,
		GrantType:              consent.GrantDelegated,
		Scopes:                 []string{"Files.ReadWrite.All"},
		IsActive:               true,
	}

	findingsRepo := &fakeFindingsRepo{}
	deps := assess.Deps{
		AccountsRepo:  &fakeAccountsRepo{},
		ConsentApps:   &fakeConsentApps{apps: []*consent.ConsentedApplication{app}},
		ConsentGrants: &fakeConsentGrants{grants: []*consent.ConsentGrant{grant}},
		OwnerTerminus: &fakeOwnerTerminus{byPrincipal: map[uuid.UUID]string{consenterPID: "terminated_human"}},
		RunsRepo:      &fakeRunsRepo{},
		FindingsRepo:  findingsRepo,
		Store:         store,
		Dispatcher:    dispatcher,
		OwnerResolver: emptyOwnerResolver(),
		Log:           nopLog(),
	}

	result, err := assess.New(deps)(assess.Args{TriggeredBy: "test"}, actionCtx())
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if result.ConsentEvaluated != 2 {
		t.Errorf("ConsentEvaluated = %d, want 2 (1 app + 1 grant)", result.ConsentEvaluated)
	}
	if result.FindingsCreated == 0 {
		t.Fatalf("FindingsCreated = 0, want > 0; kinds=%v", kindList(findingsRepo.inserted))
	}

	var got *findings.Finding
	for _, f := range findingsRepo.inserted {
		if f.Kind == "consent_owner_terminated" {
			got = f
			break
		}
	}
	if got == nil {
		t.Fatalf("no consent_owner_terminated finding; kinds=%v", kindList(findingsRepo.inserted))
	}
	if got.TargetType == nil || *got.TargetType != findings.TargetConsentGrant {
		t.Errorf("TargetType = %v, want %s", got.TargetType, findings.TargetConsentGrant)
	}
	if got.TargetID == nil || *got.TargetID != grantID {
		t.Errorf("TargetID = %v, want %s", got.TargetID, grantID)
	}
	if got.PrincipalID == nil || *got.PrincipalID != consenterPID {
		t.Errorf("PrincipalID = %v, want %s (the consenting principal)", got.PrincipalID, consenterPID)
	}
}

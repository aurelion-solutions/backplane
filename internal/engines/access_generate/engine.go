// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/integrations/applications"
	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/aurelion-solutions/backplane/internal/inventory/capabilities"
	"github.com/aurelion-solutions/backplane/internal/inventory/employments"
	"github.com/aurelion-solutions/backplane/internal/inventory/initiatives"
	"github.com/aurelion-solutions/backplane/internal/inventory/org_units"
	"github.com/aurelion-solutions/backplane/internal/inventory/principals"
)

// Engine is the access_generate orchestration object. It is
// stateless beyond its dependencies and safe for concurrent use.
type Engine struct {
	deps Deps
}

// Deps wires the engine to its collaborators.
//
// Cartridges + BundleRef: where inheritance rules live. We call
// `Cartridges.Policies(BundleRef)` and filter by
// `Mechanism == MechanismInheritance`. The cartridges layer owns
// the question of "where do these files actually come from" — we
// never walk the filesystem directly.
//
// Initiatives + Accounts: persistence boundaries the engine writes
// to. Both are called inside a single transaction opened by
// Recompute.
//
// Events: MQ sink for `inventory.initiative.created` /
// `inventory.initiative.tombstoned`. Published after the transaction
// commits, so an emit failure cannot leave the DB in an inconsistent
// state with the message log.
type Deps struct {
	Cartridges  cartridges.Provider
	BundleRef   cartridges.Ref
	Initiatives initiatives.Repository
	Accounts    accounts.Repository

	// Read-side repositories the inheritance resolver needs to walk
	// from a principal id down to (OU DN, application id, capability
	// id). All are read-only here — the engine never mutates these
	// rows.
	Principals   principals.Repository
	Employments  employments.Repository
	OrgUnits     org_units.Repository
	Applications applications.Repository
	Capabilities capabilities.Repository

	DB     *bun.DB
	Events EventSink
	Actor  string // value to stamp into `Initiative.Actor` on Create
}

// RecomputeFilter narrows what Recompute touches. When empty, the
// entire (principal, ∀ applications, ∀ capabilities) scope is
// rebuilt.
//
// Filters are advisory at the diff stage: planned initiatives
// outside the filter are dropped before the diff is computed, so
// they stay untouched in the DB. This lets a "review account X"
// trigger update only the slice it cares about without recomputing
// the whole principal.
type RecomputeFilter struct {
	ApplicationID *uuid.UUID
	CapabilityID  *uuid.UUID
}

// RecomputeResult is what Recompute hands back to the caller.
//
// The MQ events list is what *will* be emitted after the surrounding
// transaction commits — callers that want to inspect the diff
// without performing the publish (tests, dry-run) can read it here.
type RecomputeResult struct {
	Created    []uuid.UUID
	Tombstoned []uuid.UUID
	Events     []PendingEvent
}

// New constructs the engine.
func New(d Deps) (*Engine, error) {
	if d.Cartridges == nil {
		return nil, errors.New("access_generate: Cartridges required")
	}
	if d.BundleRef.ID == "" {
		return nil, errors.New("access_generate: BundleRef.ID required")
	}
	if d.Initiatives == nil {
		return nil, errors.New("access_generate: Initiatives required")
	}
	if d.Accounts == nil {
		return nil, errors.New("access_generate: Accounts required")
	}
	if d.Principals == nil {
		return nil, errors.New("access_generate: Principals required")
	}
	if d.Employments == nil {
		return nil, errors.New("access_generate: Employments required")
	}
	if d.OrgUnits == nil {
		return nil, errors.New("access_generate: OrgUnits required")
	}
	if d.Applications == nil {
		return nil, errors.New("access_generate: Applications required")
	}
	if d.Capabilities == nil {
		return nil, errors.New("access_generate: Capabilities required")
	}
	if d.DB == nil {
		return nil, errors.New("access_generate: DB required")
	}
	if d.Events == nil {
		return nil, errors.New("access_generate: Events required")
	}
	if d.Actor == "" {
		return nil, errors.New("access_generate: Actor required")
	}
	return &Engine{deps: d}, nil
}

// Recompute is the single entry point. Every trigger (Journey
// pipeline action, beat-scheduled pass, ad-hoc REST call) reduces to
// `Recompute(ctx, principalID, filter)`.
//
// Steps:
//
//  1. Collect planned initiatives from each source: inheritance
//     (implemented), requested (stub), delegated (stub).
//  2. Filter planned set by RecomputeFilter so out-of-scope
//     initiatives are not touched.
//  3. Read the principal's current active initiatives, filtered by
//     the same scope.
//  4. Diff: anything in planned-but-not-current → Create; anything
//     in current-but-not-planned → Tombstone.
//  5. Recompute desired_state on affected accounts (and grants, when
//     the grants slice lands).
//  6. Stage MQ events; publish after commit.
//
// Steps 1-5 run inside a single transaction. Step 6 runs after the
// commit returns. The transaction handle is threaded through the
// repository calls so they all use the same atomic snapshot.
func (e *Engine) Recompute(ctx context.Context, principalID uuid.UUID, f RecomputeFilter) (*RecomputeResult, error) {
	result := &RecomputeResult{}
	correlationID := newCorrelationID()
	err := e.deps.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		planned, err := e.collectPlanned(ctx, tx, principalID, f)
		if err != nil {
			return fmt.Errorf("collect planned: %w", err)
		}
		current, err := e.loadCurrentActive(ctx, tx, principalID, f)
		if err != nil {
			return fmt.Errorf("load current: %w", err)
		}
		toCreate, toTombstone := diff(planned, current)

		for _, p := range toCreate {
			ini, err := e.createInitiative(ctx, tx, principalID, p)
			if err != nil {
				return fmt.Errorf("create initiative: %w", err)
			}
			result.Created = append(result.Created, ini.ID)
			result.Events = append(result.Events, eventCreated(ini, correlationID))
		}
		for _, c := range toTombstone {
			if err := e.deps.Initiatives.Tombstone(ctx, tx, c.ID); err != nil {
				return fmt.Errorf("tombstone initiative %s: %w", c.ID, err)
			}
			result.Tombstoned = append(result.Tombstoned, c.ID)
			result.Events = append(result.Events, eventTombstoned(c, correlationID))
		}

		// TODO: recompute desired_state on affected accounts (and
		// grants when the lake-grants slice is ready). Skipped for
		// the skeleton so the transaction stays clean; the
		// inheritance-only walk is testable without this step.

		return nil
	})
	if err != nil {
		return nil, err
	}

	// MQ publish happens after commit so a broker outage cannot
	// roll back persisted state.
	for _, ev := range result.Events {
		env, err := events.NewEnvelope(ev.Envelope)
		if err != nil {
			return result, fmt.Errorf("build envelope %s: %w", ev.Topic, err)
		}
		if err := e.deps.Events.Emit(ctx, env); err != nil {
			return result, fmt.Errorf("emit %s: %w", ev.Topic, err)
		}
	}
	return result, nil
}

// collectPlanned merges contributions from each source into the
// flat planned set. Sources that are not implemented yet contribute
// nothing — see requested.go / delegated.go for the placeholder
// shape.
func (e *Engine) collectPlanned(ctx context.Context, tx bun.IDB, principalID uuid.UUID, f RecomputeFilter) ([]plannedInitiative, error) {
	out := []plannedInitiative{}

	fromInheritance, err := e.resolveInheritanceInitiatives(ctx, tx, principalID, f)
	if err != nil {
		return nil, fmt.Errorf("inheritance: %w", err)
	}
	out = append(out, fromInheritance...)

	fromRequested, err := e.resolveRequestedInitiatives(ctx, tx, principalID, f)
	if err != nil {
		return nil, fmt.Errorf("requested: %w", err)
	}
	out = append(out, fromRequested...)

	fromDelegated, err := e.resolveDelegatedInitiatives(ctx, tx, principalID, f)
	if err != nil {
		return nil, fmt.Errorf("delegated: %w", err)
	}
	out = append(out, fromDelegated...)

	return applyFilter(out, f), nil
}

// loadCurrentActive returns the principal's currently-active
// initiatives that fall inside the filter scope. Used as the "right
// side" of the diff.
func (e *Engine) loadCurrentActive(ctx context.Context, tx bun.IDB, principalID uuid.UUID, f RecomputeFilter) ([]*initiatives.Initiative, error) {
	list, _, err := e.deps.Initiatives.List(ctx, tx, initiatives.ListFilter{
		PrincipalID:   &principalID,
		ApplicationID: f.ApplicationID,
		CapabilityID:  f.CapabilityID,
		ActiveOnly:    true,
	})
	return list, err
}

// createInitiative persists one plannedInitiative as a new active
// row. Actor / Justification carry the engine's signature so the
// audit trail says where the row came from.
func (e *Engine) createInitiative(ctx context.Context, tx bun.IDB, principalID uuid.UUID, p plannedInitiative) (*initiatives.Initiative, error) {
	ini := &initiatives.Initiative{
		ID:            uuid.New(),
		PrincipalID:   principalID,
		ApplicationID: p.ApplicationID,
		CapabilityID:  p.CapabilityID,
		Kind:          p.Kind,
		Justification: p.Justification,
		Actor:         e.deps.Actor + ":" + p.SourceRuleID,
	}
	if err := e.deps.Initiatives.Create(ctx, tx, ini); err != nil {
		return nil, err
	}
	return ini, nil
}

// applyFilter drops planned entries that fall outside the scope of
// the filter. Comparing on the same shape the repository's ListFilter
// uses keeps the diff symmetric on both sides.
func applyFilter(in []plannedInitiative, f RecomputeFilter) []plannedInitiative {
	if f.ApplicationID == nil && f.CapabilityID == nil {
		return in
	}
	out := in[:0]
	for _, p := range in {
		if f.ApplicationID != nil && p.ApplicationID != *f.ApplicationID {
			continue
		}
		if f.CapabilityID != nil {
			if p.CapabilityID == nil || *p.CapabilityID != *f.CapabilityID {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

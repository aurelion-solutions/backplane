// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package run

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/engines/access_generate"
)

// Deps is the composition-root injection set. The action does not
// own any persistence layer of its own — it dispatches to the
// pre-built engine, which carries every repository it needs.
type Deps struct {
	Engine *access_generate.Engine
}

// New builds the handler closure with deps bound.
func New(deps Deps) registry.Handler[Args, Result] {
	return func(args Args, ctx registry.ActionContext) (Result, error) {
		principalID, err := uuid.Parse(args.PrincipalID)
		if err != nil {
			return Result{}, fmt.Errorf("access_generate.run: parse principal_id %q: %w", args.PrincipalID, err)
		}

		filter := access_generate.RecomputeFilter{}
		if args.ApplicationID != "" {
			id, err := uuid.Parse(args.ApplicationID)
			if err != nil {
				return Result{}, fmt.Errorf("access_generate.run: parse application_id %q: %w", args.ApplicationID, err)
			}
			filter.ApplicationID = &id
		}
		if args.CapabilityID != "" {
			id, err := uuid.Parse(args.CapabilityID)
			if err != nil {
				return Result{}, fmt.Errorf("access_generate.run: parse capability_id %q: %w", args.CapabilityID, err)
			}
			filter.CapabilityID = &id
		}

		out, err := deps.Engine.Recompute(ctx.Ctx, principalID, filter)
		if err != nil {
			return Result{}, fmt.Errorf("access_generate.run: %w", err)
		}
		return Result{
			CreatedCount:    len(out.Created),
			TombstonedCount: len(out.Tombstoned),
			EventsEmitted:   len(out.Events),
		}, nil
	}
}

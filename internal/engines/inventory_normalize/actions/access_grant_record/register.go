// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_grant_record

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register hooks the inventory_normalize.access_grant_record action
// into r. Composition roots (cmd/backplane, cmd/worker) call this at
// startup.
//
// The action is marked idempotent: every upsert is keyed by lineage
// (source_grant_external_id, source_capability_mapping_id), and the
// projector is a pure function, so a re-run over the same lake batch
// with the same mapping set converges to the same set of rows.
func Register(r *registry.Registry, deps Deps) {
	registry.MustRegister[Args, Result](r, "inventory_normalize", "access_grant_record", true, New(deps))
}

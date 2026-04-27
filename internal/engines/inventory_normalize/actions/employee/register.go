// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register hooks the inventory_normalize.employee action into r.
//
// Idempotency: every step is keyed by either
// (source, source_record_external_id) on the match row or
// (person_id, key) on person_attributes. Re-running the action over
// the same lake batch is a no-op.
func Register(r *registry.Registry, deps Deps) {
	registry.MustRegister[Args, Result](r, "inventory_normalize", "employee", true, New(deps))
}

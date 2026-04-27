// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orgunit

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register hooks the inventory_normalize.orgunit action into r.
//
// Idempotent: every node is upserted by external_id; replaying the
// same tree against the same active rules converges to the same set
// of rows.
func Register(r *registry.Registry, deps Deps) {
	registry.MustRegister[Args, Result](r, "inventory_normalize", "orgunit", true, New(deps))
}

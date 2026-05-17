// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package person

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register hooks the inventory_normalize.person action into r.
func Register(r *registry.Registry, deps Deps) {
	registry.MustRegister[Args, Result](r, "inventory_normalize", "person", false, New(deps))
}

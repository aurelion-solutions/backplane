// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package account

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register hooks the inventory_normalize.account action into r.
// Composition roots (cmd/backplane, cmd/worker) call this at startup
// so the loader can reference it and the runner can dispatch it.
//
// The action is marked idempotent: the upsert is keyed by
// (application_id, username), and the bun INSERT … ON CONFLICT path
// converges deterministically regardless of how many times the same
// batch is replayed.
func Register(r *registry.Registry, deps Deps) {
	registry.MustRegister[Args, Result](r, "inventory_normalize", "account", true, New(deps))
}

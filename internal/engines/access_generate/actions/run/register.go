// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package run

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register hooks the access_generate.run action into r.
// Composition roots call this at startup so the loader can reference
// it and the runner can dispatch it.
//
// Not idempotent: each invocation reads the current snapshot and
// produces a diff. Two back-to-back runs with the same args are not
// guaranteed to be no-ops if state has shifted between them.
func Register(r *registry.Registry, deps Deps) {
	registry.MustRegister[Args, Result](r, "access_generate", "run", false, New(deps))
}

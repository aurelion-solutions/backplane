// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package assess

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register hooks the policy_assessment.assess action into r.
// Composition roots (cmd/worker) call this at startup so the loader
// can reference it and the runner can dispatch it.
//
// The action is NOT marked idempotent. Each invocation creates a
// fresh assessment_run row; re-running the same step produces a new
// run row plus reused findings (the unique constraint on the
// evidence tuple folds duplicates).
func Register(r *registry.Registry, deps Deps) {
	registry.MustRegister[Args, Result](r, "policy_assessment", "assess", false, New(deps))
}

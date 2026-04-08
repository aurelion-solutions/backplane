// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register wires noop.echo and noop.sleep into r. Composition roots
// (cmd/backplane, cmd/worker) call this at startup so the loader can
// reference them and the runner can dispatch them.
func Register(r *registry.Registry) {
	registry.MustRegister[EchoArgs, EchoResult](r, "noop", "echo", true, echo)
	registry.MustRegister[SleepArgs, SleepResult](r, "noop", "sleep", true, sleep)
}

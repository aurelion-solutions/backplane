// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import "github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"

// Register wires the noop primitive actions into r. Composition roots
// (cmd/backplane, cmd/worker) call this at startup so the loader can
// reference them and the runner can dispatch them.
//
// Idempotency:
//   - echo, sleep, constant, fail are idempotent — a retried run
//     produces the same effect.
//   - emit is NOT idempotent — every dispatch publishes a fresh
//     envelope with a new event_id, so a retry creates a duplicate.
func Register(r *registry.Registry) {
	registry.MustRegister[EchoArgs, EchoResult](r, "noop", "echo", true, echo)
	registry.MustRegister[SleepArgs, SleepResult](r, "noop", "sleep", true, sleep)
	registry.MustRegister[FailArgs, FailResult](r, "noop", "fail", true, failAction)
	registry.MustRegister[ConstantArgs, ConstantResult](r, "noop", "constant", true, constant)
	registry.MustRegister[EmitArgs, EmitResult](r, "noop", "emit", false, emit)
}

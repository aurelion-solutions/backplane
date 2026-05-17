// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package registry

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/aurelion-solutions/backplane/internal/core/events"
)

// ActionContext is the immutable per-step context the runner injects
// into every action handler.
//
// The runner owns the transaction:
//   - Tx is the live *bun.Tx; the action MUST NOT call Commit / Rollback
//     and MUST NOT open its own transaction.
//   - On handler error the runner rolls back Tx and persists step
//     failure in a fresh transaction.
//
// Events is the domain-event sink. Actions that publish envelopes use it;
// actions that do not are unaffected. May be nil in tests that drive a
// handler directly.
type ActionContext struct {
	Ctx           context.Context
	Tx            bun.IDB
	Log           *slog.Logger
	Events        events.Sink
	PipelineRunID uuid.UUID
	StepRunID     uuid.UUID
	Attempt       int
	WorkerID      string
}

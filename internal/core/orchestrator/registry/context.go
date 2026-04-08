// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package registry

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ActionContext is the immutable per-step context the runner injects
// into every action handler.
//
// The runner owns the transaction:
//   - Tx is the live *bun.Tx; the action MUST NOT call Commit / Rollback
//     and MUST NOT open its own transaction.
//   - On handler error the runner rolls back Tx and persists step
//     failure in a fresh transaction.
type ActionContext struct {
	Ctx           context.Context
	Tx            bun.IDB
	Log           *slog.Logger
	PipelineRunID uuid.UUID
	StepRunID     uuid.UUID
	Attempt       int
	WorkerID      string
}

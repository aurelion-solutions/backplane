// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package runner is the orchestrator's work loop. It runs inside
// cmd/worker — one slot per goroutine, N goroutines per process —
// and claims pending runs from Postgres via SELECT ... FOR UPDATE
// SKIP LOCKED.
//
// Design invariants:
//
//   - Three-transaction-per-step protocol. Tx A claims the run. Tx B
//     persists the StepRun row + commits it (so a later Tx that records
//     failure can see it). Tx C runs the action under bun.Tx; on
//     success the same Tx commits the success transition. On action
//     error Tx C is rolled back and a fresh Tx D writes the failure.
//   - The runner is the sole orchestrator code that calls Commit/
//     Rollback on bun.Tx; Service / Repository never do.
//   - A heartbeat goroutine refreshes pipeline_runs.last_heartbeat_at
//     every 3 s while the action runs and watches for a cancelling
//     status flip; when it spots one, it triggers a Go context cancel
//     so the action handler can unwind.
//   - On Service.IsTerminal failure the runner's reclaim sweep
//     (which runs at the head of every loop iteration) picks up any
//     stale run left behind by a crashed peer.
package runner

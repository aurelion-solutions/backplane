// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package orchestrator owns the three pipeline state tables
// (pipeline_runs, step_runs, pipeline_event_waiters) and the Service
// that is the sole writer to them.
//
// Sub-packages:
//
//	grammar/   embedded JSON Schema for pipeline YAML
//	loader/    YAML parser + validator
//	registry/  in-memory action registry
//	runner/    (Step 6) work-loop that claims runs from Postgres
//
// Invariant: only this package's Service mutates the three tables.
// Engines read their own data but MUST NOT write to orchestrator
// state. Every status-changing UPDATE WHERE-guards on the expected
// source status; a zero-rowcount triggers refresh-and-branch logic
// (cancel-vs-complete race handling), never silent retries.
package orchestrator

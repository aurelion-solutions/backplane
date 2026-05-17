// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package policy_assessment_runs persists one row per policy
// assessment pass.
//
// An assessment run records the lifecycle of a single pass over the
// policy catalogue: triggered by manual API call, scheduled cron, or
// the worker policy-assessment action. The row carries status, scope
// (principal / application when narrowed), counters (total findings,
// per-severity breakdown, created vs reused), and the operator
// timestamps required to audit when a pass started and finished.
//
// Findings produced during a pass reference the assessment run via
// FK; the run cannot be deleted while findings still point at it
// (ON DELETE RESTRICT).
package policy_assessment_runs

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package assess is the policy_assessment.assess action — the
// pipeline-runnable unit of work that evaluates the active policy
// catalogue against the current inventory snapshot and persists
// findings.
//
// One invocation = one assessment run row. The action walks the
// accounts snapshot, builds Facts for each account, dispatches every
// applicable policy through the policy_assessment engine, and writes
// the resulting Decision into a finding row. Idempotency is enforced
// by the DB unique constraint on the finding evidence tuple — a
// re-run that produces the same finding for the same account+policy
// reuses the existing row.
//
// The action is registered into the orchestrator action registry by
// the worker composition root, exactly like inventory_normalize
// actions. The pipeline cartridge owns the trigger (schedule / mq /
// manual) and the args mapping.
package assess

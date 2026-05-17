// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package findings persists one row per detected violation or anomaly
// emitted by a policy assessment pass.
//
// Each finding belongs to exactly one assessment run and references at
// least one of (principal, account) — orphan-account-style findings
// carry an account anchor without a principal. The policy that
// produced the finding is referenced via policy_id when available;
// legacy/anonymous emissions can omit it.
//
// Kind is a free-form short label owned by the emitting policy
// (e.g. "orphan_access", "terminated_access", "sod"). The vocabulary
// is intentionally open — new kinds arrive through new cartridges
// without any schema change.
//
// evidence_hash is the canonical idempotency key: the same finding
// surfaced by the same policy against the same anchors on a later
// assessment run reuses the existing row (or surfaces as a no-op)
// rather than duplicating. Severity follows the kernel enum
// (critical / high / medium / low) and is enforced by a CHECK
// constraint.
//
// active_mitigation_id and proposed_mitigation_id are plain UUID
// columns without a foreign key; the mitigations slice will add the
// constraint when it ships.
package findings

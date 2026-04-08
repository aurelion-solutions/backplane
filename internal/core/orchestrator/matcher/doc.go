// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package matcher is the MQ event consumer that drives two effects:
//
//  1. Waiter resolution. A wait_for_event step parked in
//     pipeline_event_waiters whose match JSONB is contained in the
//     incoming event payload is resolved via the same
//     Service.ResolveEventWaiter the HITL endpoint uses.
//  2. MQ-trigger firing. Pipelines with type=mq triggers whose
//     routing_key matches the delivery and whose match predicate is
//     contained in the payload are started via
//     Service.CreateRun(TriggerMQ).
//
// Effects (1) and (2) run in independent Postgres transactions: a
// failure in one does not roll back the other.
//
// Cluster-wide there is at most one active matcher. The active
// process holds a session-level pg_advisory_lock on a dedicated
// connection (key 0x4155_5245_4C4D_4154 = "AURELMAT"); siblings that
// cannot acquire it become warm standbys that retry every second.
package matcher

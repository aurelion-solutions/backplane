// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package beat is the periodic scheduler / timeout sweeper.
//
// Two responsibilities:
//
//  1. Fire schedule-triggered pipelines whose cron / every window
//     has elapsed since the last fire, with same-window dedupe.
//  2. Sweep pipeline_event_waiters whose expires_at is in the past
//     and transition the parked step + run to failed_timeout.
//
// Multi-replica safety: each tick acquires pg_try_advisory_lock on a
// per-tick basis; a sibling that loses the lock skips the tick. The
// lock key is the 64-bit integer 0x4155_5245_4C42_4541 ("AURELBEA7").
package beat

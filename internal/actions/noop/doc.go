// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package noop ships two trivial actions used by smoke pipelines and
// the integration test suite:
//
//	noop.echo  — copies the message arg into the result
//	noop.sleep — sleeps for the requested duration (heartbeat / cancel
//	             test harness)
//
// They have no business meaning and never touch the database; their
// purpose is to exercise the orchestrator runner end-to-end without
// pulling in a real engine.
package noop

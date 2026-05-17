// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package noop ships the pipeline-shape primitive actions that belong
// to the orchestrator itself rather than to any engine:
//
//	noop.echo     — copies the message arg into the result
//	noop.sleep    — sleeps for the requested duration (heartbeat /
//	                cancel test harness, deliberate pacing)
//	noop.fail     — raises a deliberate handler error with a supplied
//	                message; populates the Failed sidebar bucket and
//	                exercises the reclaim / retry paths
//	noop.constant — returns an arbitrary JSON object, useful to stub
//	                a producer step before the real action exists
//	noop.emit     — publishes a domain envelope through
//	                ActionContext.Events; lets a pipeline emit the
//	                event a downstream wait_for_event step waits on
//
// They have no business meaning and never touch a business table.
// emit is the only one with a side effect; the rest are pure.
package noop

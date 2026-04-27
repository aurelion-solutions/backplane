// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_discover

const (
	EventActorComponent = "engines.inventory_discover"

	// Lifecycle events this engine emits.
	EventRunDispatched = "inventory.discover.run_dispatched"
	EventRunCompleted  = "inventory.discover.run_completed"
	EventRunFailed     = "inventory.discover.run_failed"

	// Event names this engine listens for, emitted by connectors as
	// they progress. Connectors stamp the same correlation_id the
	// dispatch carried so the subscriber can locate the run.
	EventConnectorStarted   = "connector.discover.started"
	EventConnectorCompleted = "connector.discover.completed"
	EventConnectorFailed    = "connector.discover.failed"
)

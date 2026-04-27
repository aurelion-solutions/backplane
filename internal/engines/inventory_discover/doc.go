// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package inventory_discover orchestrates pull-side ingest.
//
// The engine creates a DiscoverRun row, dispatches a "go discover"
// command to the connector via the existing connectors RPC channel
// (fire-and-forget — the actual reply is async), and then waits for
// completion events the connector emits as it streams records into
// the lake through inventory_ingest.
//
// What this engine is NOT:
//   - it does not stream records itself,
//   - it does not write to the lake,
//   - it does not validate or normalise record payloads.
//
// Those concerns belong to the connector (which publishes records
// directly into the aurelion.ingest MQ exchange) and to
// inventory_ingest (which dedupes and writes). This engine just
// tracks lifecycle: dispatched → running → completed / failed /
// timed_out.
//
// The correlation_id stamped on the DiscoverRun travels with every
// record the connector publishes for this run, so an operator can
// look up "how many records did discover X bring in" by counting
// audit rows in inventory_ingest with the same correlation_id.
package inventory_discover

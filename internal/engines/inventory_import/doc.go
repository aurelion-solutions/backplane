// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package inventory_import is the synchronous CSV-import façade
// over inventory_ingest + inventory_normalize.
//
// One HTTP request runs both phases in order against the same
// (source, dataset_type, records) input:
//
//  1. inventory_ingest.Service.Process writes the batch into the
//     data lake and persists the audit row. The MQ event is
//     SUPPRESSED here — see SkipEvent on inventory_ingest.Request —
//     because we are about to normalize synchronously and do not
//     want the async pipeline racing against the same lake batch.
//
//  2. The matching inventory_normalize action is dispatched via
//     the orchestrator registry with the freshly produced
//     batch_id + lake_ref. The whole call lives inside one bun
//     transaction so a normalize failure rolls back every PG write
//     that the action did.
//
// The Lens CSV demo wizard targets this endpoint instead of the
// underlying /ingest one — it wants determinism (one request, one
// response, "X rows imported and normalized") more than it wants
// the async pipeline's decoupling.
//
// What this package does NOT do:
//
//   - It is not a general orchestration layer. Each request runs
//     exactly one dataset_type. Multiple datasets = multiple calls.
//   - It does not retry. A failed Import surfaces the error; the
//     caller decides whether to retry.
//   - It does not own a separate audit table — the ingest audit row
//     and the normalize action's own logs are the trail.
package inventory_import

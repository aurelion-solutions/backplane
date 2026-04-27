// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package inventory_ingest is the single writer into the data lake.
//
// The engine is a pure function: callers hand it (source, dataset_type,
// records) and the engine hashes each record, anti-joins against the
// lake's latest revision per external_id, writes only the new and
// changed records as one lake batch, persists an audit row, and emits
// inventory.ingest.batch_received.
//
// It owns NO transport. It is invoked by:
//   - the thin POST /ingest HTTP handler (CLI / upstream callers),
//   - the MQ consumer goroutine in the composition root (connectors),
//   - the discover engine when a pull completes.
//
// Three things every record must carry:
//   - external_id — the source-side identifier; required for dedup.
//   - any other source fields (verbatim) — become the lake "payload".
//   - nothing else; backplane adds meta (hash, committed_at,
//     correlation_id) at write time.
//
// Records that share an external_id + hash with the latest revision
// in the lake are skipped silently. This is what makes a 50K-row
// pull cheap when only 50 things actually changed.
package inventory_ingest

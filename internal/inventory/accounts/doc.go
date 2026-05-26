// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package accounts owns the Account entity — one row per provider
// user-mailbox in a given Application. Natural key is
// (application_id, username); external_id and source are recorded for
// traceability back to the connector batch that produced the row.
//
// Accounts are pure inventory. Decisions about WHO stands behind an
// Account (employee, workload, customer) belong to a separate
// account → principal matcher engine, which runs AFTER normalize.
// Decisions about WHAT this Account can do live in
// inventory_normalize.access_grant_record (CapabilityGrant).
package accounts

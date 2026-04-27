// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package employee_provider_mappings owns the per-provider rules
// used by inventory_normalize.employee to resolve raw EmployeeRecords
// into canonical Persons via determinator + upstream traversal.
//
// One row per (provider, record_key). is_determinator=TRUE marks the
// provider as authoritative — it may CREATE new Persons. allow_upstream
// =TRUE marks a secondary attribute — it may ATTACH the record to an
// existing Person but never creates one. Pure reference data,
// admin-managed.
package employee_provider_mappings

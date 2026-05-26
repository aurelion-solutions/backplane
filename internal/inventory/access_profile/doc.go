// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package access_profile is a read-only projection over the inventory
// layer: given a Person, it assembles the full human-access picture in
// one nested document — employment periods, and per application the
// accounts the person holds (with their capability grants) plus the
// initiatives (justifications) that authorise that access, including
// their validity windows.
//
// It owns no table and emits no events. It walks the existing
// inventory spine —
//
//	person → employments → employment-principals → accounts → grants
//	                                              ↘ initiatives
//
// — joining the catalog (capabilities, scope keys, applications) for
// human-facing labels. The account → principal edge it relies on is
// accounts.principal_id (the assignment edge).
//
// HTTP surface: GET /api/v0/persons/{id}/access-profile. Read-only;
// safe on prefetch/HEAD — never writes.
package access_profile

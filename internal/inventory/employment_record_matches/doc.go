// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package employment_record_matches owns the lineage record connecting
// one raw lake EmployeeRecord (by source + source_record_external_id)
// to one Employment row.
//
// Multiple matches per Employment are allowed — both an HRIS record
// and an AD record can be attached to the same Employment when both
// describe the same job from different angles. Uniqueness is on
// (source, source_record_external_id), the lake natural key.
package employment_record_matches

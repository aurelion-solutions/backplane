// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workload_lineage

import "errors"

// ErrWorkloadNotFound is returned when the requested workload does not
// exist in the inventory.
var ErrWorkloadNotFound = errors.New("workload_lineage: workload not found")

// ErrReaderNotFound is the canonical sentinel that port adapters must
// return when the underlying repository returns a not-found error.
// The resolver checks for this sentinel via errors.Is — adapters wrap
// their slice-specific not-found errors with this value.
var ErrReaderNotFound = errors.New("workload_lineage: reader not found")

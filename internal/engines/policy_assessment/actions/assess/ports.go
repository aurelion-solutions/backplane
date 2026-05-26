// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package assess

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ResolvedChain is the assess-local minimal view of a resolved
// ownership chain. The composition root adapter maps the full
// workload_lineage.OwnershipChain to this shape so that the assess
// action never imports the workload_lineage package.
type ResolvedChain struct {
	WorkloadID          uuid.UUID
	Terminus            string
	OwnerPersonID       string
	OwnerLabel          string
	LastTerminationDate *time.Time
}

// LineageResolver resolves the ownership chain for one workload.
// Declared locally so assess never imports workload_lineage.
type LineageResolver interface {
	Resolve(ctx context.Context, workloadID uuid.UUID) (ResolvedChain, error)
}

// OwnerTerminusResolver resolves the ownership terminus of a single
// principal — "active_human", "terminated_human", or "" when it cannot
// be resolved (no body, unknown principal). Implemented in the
// composition root over the principals / employments / lineage repos so
// assess never imports them directly. Powers the secret pass's
// owner-terminated check.
type OwnerTerminusResolver interface {
	Resolve(ctx context.Context, principalID uuid.UUID) (string, error)
}

// SnapshotWriter persists the lineage snapshot for one workload.
// The adapter in cmd/worker re-resolves the full OwnershipChain
// internally and writes it, keeping assess free of the full chain type.
// This is the ONLY path through which snapshots are written (R1) —
// never the GET lineage endpoint.
type SnapshotWriter interface {
	RecordSnapshot(ctx context.Context, workloadID uuid.UUID) error
}

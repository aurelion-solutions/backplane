// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package workload_lineage derives a workload's ownership chain to the
// originating human; append-only lineage snapshots for replay.
//
// The resolver walks: workload → owning employment → person → all of
// that person's employments, and classifies the chain terminus as one
// of: active_human, terminated_human, unowned, or broken_link.
//
// Snapshot writes happen exclusively in the policy_assessment assess
// pass (cmd/worker). The GET /workloads/:id/lineage endpoint is
// read-only and never writes a snapshot.
package workload_lineage

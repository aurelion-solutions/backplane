// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package principals

import (
	"context"
	"fmt"

	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/google/uuid"
)

// EmploymentState is the slice of an Employment the status-derivation
// needs. The employments-slice adapter at the composition root
// implements it. Employment status is tenant-defined free text — the
// derivation simply mirrors Employment.code onto the Principal.
type EmploymentState interface {
	// EmploymentCode returns the current `code` of an employment row
	// and whether the row exists.
	EmploymentCode(ctx context.Context, id uuid.UUID) (code string, exists bool, err error)
}

// WorkloadState is the workload-slice view needed for status
// derivation. Workload status here covers ONLY lifecycle (active vs
// expired). Operational lock lives on Principal.is_locked, not here.
type WorkloadState interface {
	WorkloadExists(ctx context.Context, id uuid.UUID) (bool, error)
}

// CustomerState is the customer-slice view needed for status
// derivation. After is_locked moved to Principal, the only signal
// customers contributes is email_verified.
type CustomerState interface {
	CustomerState(ctx context.Context, id uuid.UUID) (CustomerStateView, error)
}

// CustomerStateView is the projection of a Customer that drives
// principal status derivation.
type CustomerStateView struct {
	Exists        bool
	EmailVerified bool
}

// BodySources bundles the three body-state probes.
type BodySources struct {
	Employments EmploymentState
	Workloads   WorkloadState
	Customers   CustomerState
}

// DeriveStatus returns the principal status that should hold given the
// current state of the underlying body:
//
//   - employment → Employment.code verbatim; "terminated" when the
//     row is gone
//   - workload   → active when the workload row exists, expired when
//     it has been removed
//   - customer   → active when email_verified, registered otherwise.
//     suspended / banned / deletion_requested are not
//     reached via derivation; they are set explicitly by
//     the operations that put a customer there.
//
// Lock is intentionally NOT part of this function — it is a separate
// axis stored directly on Principal.
func DeriveStatus(ctx context.Context, sources BodySources, kind shared.PrincipalKind, bodyID uuid.UUID) (string, error) {
	switch kind {
	case shared.PrincipalKindEmployment:
		if sources.Employments == nil {
			return "active", nil
		}
		code, ok, err := sources.Employments.EmploymentCode(ctx, bodyID)
		if err != nil {
			return "", err
		}
		if !ok {
			return "terminated", nil
		}
		return code, nil

	case shared.PrincipalKindWorkload:
		if sources.Workloads == nil {
			return string(shared.WorkloadStatusActive), nil
		}
		exists, err := sources.Workloads.WorkloadExists(ctx, bodyID)
		if err != nil {
			return "", err
		}
		if !exists {
			return string(shared.WorkloadStatusExpired), nil
		}
		return string(shared.WorkloadStatusActive), nil

	case shared.PrincipalKindCustomer:
		if sources.Customers == nil {
			return string(shared.CustomerStatusRegistered), nil
		}
		v, err := sources.Customers.CustomerState(ctx, bodyID)
		if err != nil {
			return "", err
		}
		if !v.Exists {
			return string(shared.CustomerStatusBanned), nil
		}
		if v.EmailVerified {
			return string(shared.CustomerStatusActive), nil
		}
		return string(shared.CustomerStatusRegistered), nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidKind, kind)
}

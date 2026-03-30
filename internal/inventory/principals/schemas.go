// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package principals

import (
	"fmt"
	"strings"

	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/google/uuid"
)

// CreatePayload is the POST /principals body. Exactly one
// principal_*_id must be set, matching `kind`. Status is optional —
// the service derives it from body state when omitted.
type CreatePayload struct {
	ExternalID            string               `json:"external_id"`
	Kind                  shared.PrincipalKind `json:"kind"`
	PrincipalEmploymentID *uuid.UUID           `json:"principal_employment_id,omitempty"`
	PrincipalWorkloadID   *uuid.UUID           `json:"principal_workload_id,omitempty"`
	PrincipalCustomerID   *uuid.UUID           `json:"principal_customer_id,omitempty"`
	Status                *string              `json:"status,omitempty"`
}

// Validate enforces field bounds and the "exactly one body" invariant
// against `kind`.
func (p CreatePayload) Validate() error {
	if strings.TrimSpace(p.ExternalID) == "" || len(p.ExternalID) > 255 {
		return fmt.Errorf("principals: external_id must be 1..255 characters")
	}
	if !p.Kind.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidKind, p.Kind)
	}
	count := 0
	if p.PrincipalEmploymentID != nil {
		count++
	}
	if p.PrincipalWorkloadID != nil {
		count++
	}
	if p.PrincipalCustomerID != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("principals: exactly one of principal_employment_id / principal_workload_id / principal_customer_id must be set, got %d", count)
	}
	switch p.Kind {
	case shared.PrincipalKindEmployment:
		if p.PrincipalEmploymentID == nil {
			return fmt.Errorf("principals: kind=employment requires principal_employment_id")
		}
	case shared.PrincipalKindWorkload:
		if p.PrincipalWorkloadID == nil {
			return fmt.Errorf("principals: kind=workload requires principal_workload_id")
		}
	case shared.PrincipalKindCustomer:
		if p.PrincipalCustomerID == nil {
			return fmt.Errorf("principals: kind=customer requires principal_customer_id")
		}
	}
	if p.Status != nil {
		if !shared.StatusForKind(p.Kind, *p.Status) {
			return fmt.Errorf("principals: status %q not in vocabulary for kind=%q", *p.Status, p.Kind)
		}
	}
	return nil
}

// LockPayload is the POST /principals/:id/lock body.
type LockPayload struct {
	Reason *string `json:"reason,omitempty"`
}

// ListResponse is the GET /principals envelope.
type ListResponse struct {
	Items  []*Principal `json:"items"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

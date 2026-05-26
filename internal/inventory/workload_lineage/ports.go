// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workload_lineage

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// WorkloadRef is the minimal view of a workload the resolver needs.
// Owned by this slice — the composition root adapts *workloads.Workload
// to this shape so that workload_lineage does not import workloads.
type WorkloadRef struct {
	ID                uuid.UUID
	ExternalID        string
	Name              string
	OwnerEmploymentID *uuid.UUID
}

// WorkloadReader is the narrow port for loading a single workload.
type WorkloadReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*WorkloadRef, error)
}

// EmploymentRef is the minimal view of an employment the resolver needs.
type EmploymentRef struct {
	ID        uuid.UUID
	PersonID  uuid.UUID
	Code      string
	StartDate time.Time
	EndDate   *time.Time
}

// IsActiveAt reports whether the employment is active on date d.
// Semantics mirror employments.Employment.IsActiveAt: start ≤ d < end,
// NULL end = open.
func (e *EmploymentRef) IsActiveAt(d time.Time) bool {
	day := d.UTC().Truncate(24 * time.Hour)
	if day.Before(e.StartDate.UTC().Truncate(24 * time.Hour)) {
		return false
	}
	if e.EndDate == nil {
		return true
	}
	return day.Before(e.EndDate.UTC().Truncate(24 * time.Hour))
}

// EmploymentReader is the narrow port for reading employment rows.
type EmploymentReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*EmploymentRef, error)
	ListByPerson(ctx context.Context, personID uuid.UUID) ([]*EmploymentRef, error)
}

// PersonRef is the minimal view of a person the resolver needs.
type PersonRef struct {
	ID         uuid.UUID
	ExternalID string
	FullName   string
}

// PersonReader is the narrow port for reading person rows.
type PersonReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*PersonRef, error)
}

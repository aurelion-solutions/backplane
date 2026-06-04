// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workload_lineage

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// maxChainDepth is a defensive guard on recursion. The chain is a
// fixed 4-node tree (workload → employment → person → person-employments)
// so this cap will never be reached in practice.
const maxChainDepth = 8

// Resolver resolves the ownership chain for a workload.
type Resolver struct {
	workloads   WorkloadReader
	employments EmploymentReader
	persons     PersonReader
}

// NewResolver constructs a Resolver with the three narrow readers.
func NewResolver(w WorkloadReader, e EmploymentReader, p PersonReader) *Resolver {
	return &Resolver{workloads: w, employments: e, persons: p}
}

// Resolve derives the full ownership chain for workloadID.
//
// Returns ErrWorkloadNotFound when the workload does not exist.
// All other resolution failures (missing employment / person refs)
// produce a chain with Terminus = TerminusBrokenLink rather than an
// error — they represent data-quality gaps, not system errors.
func (r *Resolver) Resolve(ctx context.Context, workloadID uuid.UUID) (OwnershipChain, error) {
	now := time.Now().UTC()

	wl, err := r.workloads.GetByID(ctx, workloadID)
	if err != nil {
		if isNotFound(err) {
			return OwnershipChain{}, ErrWorkloadNotFound
		}
		return OwnershipChain{}, err
	}

	chain := OwnershipChain{
		WorkloadID: workloadID,
		ResolvedAt: now,
	}

	// Workload link — always present.
	chain.Links = append(chain.Links, ChainLink{
		Kind:  "workload",
		RefID: workloadID.String(),
		Label: wl.Name,
	})

	if len(chain.Links) >= maxChainDepth {
		chain.Terminus = TerminusBrokenLink
		return chain, nil
	}

	// No owner set → unowned.
	if wl.OwnerEmploymentID == nil {
		chain.Terminus = TerminusUnowned
		return chain, nil
	}

	emp, err := r.employments.GetByID(ctx, *wl.OwnerEmploymentID)
	if err != nil {
		if isNotFound(err) {
			chain.Terminus = TerminusBrokenLink
			return chain, nil
		}
		return OwnershipChain{}, err
	}

	empTerminated := !emp.IsActiveAt(now)
	empStart := emp.StartDate
	chain.Links = append(chain.Links, ChainLink{
		Kind:       "employment",
		RefID:      emp.ID.String(),
		Label:      emp.Code,
		Terminated: empTerminated,
		EndDate:    emp.EndDate,
		Title:      emp.Title,
		StartDate:  &empStart,
		OrgUnit:    emp.OrgUnit,
	})

	if len(chain.Links) >= maxChainDepth {
		chain.Terminus = TerminusBrokenLink
		return chain, nil
	}

	person, err := r.persons.GetByID(ctx, emp.PersonID)
	if err != nil {
		if isNotFound(err) {
			chain.Terminus = TerminusBrokenLink
			return chain, nil
		}
		return OwnershipChain{}, err
	}

	allEmps, err := r.employments.ListByPerson(ctx, person.ID)
	if err != nil {
		return OwnershipChain{}, err
	}

	// A person with zero employments is a data-quality gap: the
	// owning employment we loaded should appear in the list.
	if len(allEmps) == 0 {
		chain.Terminus = TerminusBrokenLink
		return chain, nil
	}

	// Person is terminated iff ALL their employments have ended.
	personTerminated := true
	var latestEnd *time.Time
	for _, e := range allEmps {
		if e.IsActiveAt(now) {
			personTerminated = false
		}
		if e.EndDate != nil {
			if latestEnd == nil || e.EndDate.After(*latestEnd) {
				t := *e.EndDate
				latestEnd = &t
			}
		}
	}

	var personEndDate *time.Time
	if personTerminated {
		personEndDate = latestEnd
	}

	chain.Links = append(chain.Links, ChainLink{
		Kind:       "person",
		RefID:      person.ID.String(),
		Label:      person.FullName,
		Terminated: personTerminated,
		EndDate:    personEndDate,
	})

	if personTerminated {
		chain.Terminus = TerminusTerminatedHuman
	} else {
		chain.Terminus = TerminusActiveHuman
	}

	return chain, nil
}

// isNotFound checks whether an error is a "not found" sentinel from
// any of the reader implementations. We cannot import the slice error
// values here (that would create an import cycle), so we check for
// common error substrings. The adapters in cmd/ ensure only sentinel
// errors with "not found" in their message surface here.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Use errors.As / errors.Is path for well-known sentinels.
	// Because the port types are in THIS package and the underlying
	// repo errors are in external packages we cannot import, we
	// rely on a sentinel the adapter wraps. Adapters return
	// ErrReaderNotFound from this package for not-found cases.
	return errors.Is(err, ErrReaderNotFound)
}

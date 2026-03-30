// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_records

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Outcome is the result of one Resolver pass.
//
//   - PersonID == uuid.Nil → record could not be resolved.
//   - PersonID != uuid.Nil && ViaDeterminator == true → matched
//     directly through a determinator mapping for the record's app.
//   - PersonID != uuid.Nil && ViaDeterminator == false → matched
//     transitively through an allow_upstream peer record.
//
// EmploymentID always points at the specific mask (period of work) the
// record will be bound to in the resulting EmployeeRecordMatch. When
// the resolver creates a fresh Person, it materialises one Employment
// for it as well (code="active", start_date=today, no end_date).
type Outcome struct {
	PersonID        uuid.UUID
	EmploymentID    uuid.UUID
	ViaDeterminator bool
}

// PersonAPI is the slice of the persons+employments contract the
// Resolver needs. Implemented at the composition root via an adapter
// that composes persons + employments services — the resolver never
// imports those packages directly.
type PersonAPI interface {
	// FindPersonByAttribute returns the canonical Person whose
	// PersonAttribute matches (key, value), or (uuid.Nil, false)
	// when nothing matches.
	FindPersonByAttribute(ctx context.Context, key, value string) (uuid.UUID, bool, error)

	// CreatePersonWithEmployment materialises a fresh Person seeded
	// with a single (key, value) attribute plus one Employment with
	// code="active" (start_date=today, no end_date). Returns both
	// new ids.
	//
	// Implementations are expected to:
	//   - create a Person with external_id="resolver-<uuid>",
	//     full_name="resolver-created",
	//   - upsert one PersonAttribute (key, value),
	//   - create an Employment for that Person.
	CreatePersonWithEmployment(ctx context.Context, key, value string) (personID uuid.UUID, employmentID uuid.UUID, err error)

	// PropagateAttribute upserts a non-determinator (key, value) onto
	// the canonical Person tied to a successful match. Mirrors the
	// kernel resolver's "copy mapped attributes to canonical" rule
	// but applied at the Person level.
	PropagateAttribute(ctx context.Context, personID uuid.UUID, key, value string) error

	// PrimaryEmploymentForPerson returns one currently-active
	// employment for the Person. When the resolver matches a Person
	// via determinator or upstream and the Person already exists, we
	// need to decide which mask the record binds to. The simplest
	// behaviour — and the one we ship today — is: pick the first
	// active employment. Future versions may consult a mapping rule
	// to pick a specific code (e.g. record's `role_src` value
	// chooses an Employment by code).
	//
	// Returns (uuid.Nil, false, nil) if no active employment exists.
	PrimaryEmploymentForPerson(ctx context.Context, personID uuid.UUID) (uuid.UUID, bool, error)
}

// Resolver maps an EmployeeRecord to a canonical (Person, Employment)
// pair. Pure logic — owns no state, holds the data-access interface
// and the Person API only.
type Resolver struct {
	repo    Repository
	persons PersonAPI
}

// NewResolver constructs a Resolver.
func NewResolver(repo Repository, persons PersonAPI) *Resolver {
	return &Resolver{repo: repo, persons: persons}
}

// Resolve walks the determinator → upstream sequence for recordID and
// returns the resulting Outcome. Does NOT persist EmployeeRecordMatch
// — the service is responsible for that.
func (r *Resolver) Resolve(ctx context.Context, recordID uuid.UUID) (*Outcome, error) {
	visited := map[uuid.UUID]struct{}{}
	return r.resolve(ctx, recordID, visited)
}

func (r *Resolver) resolve(ctx context.Context, recordID uuid.UUID, visited map[uuid.UUID]struct{}) (*Outcome, error) {
	if _, seen := visited[recordID]; seen {
		// cycle — bail with no match
		return nil, nil
	}
	visited[recordID] = struct{}{}

	record, err := r.repo.GetRecordByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	attrs, err := r.repo.ListRecordAttributes(ctx, recordID)
	if err != nil {
		return nil, err
	}
	attrMap := make(map[string]string, len(attrs))
	for _, a := range attrs {
		attrMap[a.Key] = a.Value
	}

	// Pass 1 — direct determinator. ANY-match: first determinator
	// mapping whose source key is present on the record wins.
	determ := true
	det, err := r.repo.ListMappings(ctx, record.ApplicationID, &determ, nil)
	if err != nil {
		return nil, err
	}
	for _, m := range det {
		val, ok := attrMap[m.EmployeeRecordKey]
		if !ok {
			continue
		}
		personID, found, err := r.persons.FindPersonByAttribute(ctx, m.PersonKey, val)
		if err != nil {
			return nil, err
		}
		var employmentID uuid.UUID
		if !found {
			pid, eid, err := r.persons.CreatePersonWithEmployment(ctx, m.PersonKey, val)
			if err != nil {
				return nil, fmt.Errorf("employee_records: resolver create-from-determinator: %w", err)
			}
			personID = pid
			employmentID = eid
		} else {
			eid, ok, err := r.persons.PrimaryEmploymentForPerson(ctx, personID)
			if err != nil {
				return nil, err
			}
			if !ok {
				// Existing Person, but no active employment to bind
				// to. Treat as "must materialise one mask" — fall
				// through to creating a fresh Person+Employment is
				// wrong (would duplicate the person); instead create
				// only the employment is not in our API contract.
				// Today we fail the resolve so the caller knows the
				// Person needs an explicit employment.
				return nil, nil
			}
			employmentID = eid
		}
		if err := r.propagate(ctx, record.ApplicationID, personID, attrMap); err != nil {
			return nil, err
		}
		return &Outcome{
			PersonID:        personID,
			EmploymentID:    employmentID,
			ViaDeterminator: true,
		}, nil
	}

	// Pass 2 — upstream peer traversal. Find peer records that share
	// at least one (key, value) under an allow_upstream mapping. For
	// each peer, recurse. First peer that resolves wins; we mark the
	// result as via_determinator=false because the bridge was upstream.
	peers, err := r.repo.FindUpstreamPeers(ctx, recordID, recordID)
	if err != nil {
		return nil, err
	}
	for _, peerID := range peers {
		outcome, err := r.resolve(ctx, peerID, visited)
		if err != nil {
			return nil, err
		}
		if outcome == nil || outcome.PersonID == uuid.Nil {
			continue
		}
		if err := r.propagate(ctx, record.ApplicationID, outcome.PersonID, attrMap); err != nil {
			return nil, err
		}
		return &Outcome{
			PersonID:        outcome.PersonID,
			EmploymentID:    outcome.EmploymentID,
			ViaDeterminator: false,
		}, nil
	}

	return nil, nil
}

// propagate copies every non-determinator mapping's value from the
// record attribute map onto the canonical Person. Skips determinator
// mappings — those drove the lookup itself.
func (r *Resolver) propagate(ctx context.Context, appID, personID uuid.UUID, attrs map[string]string) error {
	mappings, err := r.repo.ListMappings(ctx, appID, nil, nil)
	if err != nil {
		return err
	}
	for _, m := range mappings {
		if m.IsDeterminator {
			continue
		}
		val, ok := attrs[m.EmployeeRecordKey]
		if !ok {
			continue
		}
		if err := r.persons.PropagateAttribute(ctx, personID, m.PersonKey, val); err != nil {
			return err
		}
	}
	return nil
}

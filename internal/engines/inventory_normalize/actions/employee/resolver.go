// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee

import (
	"context"
	"time"

	"github.com/aurelion-solutions/backplane/internal/inventory/employee_provider_mappings"
	"github.com/aurelion-solutions/backplane/internal/inventory/persons"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// resolveOutcome reports how the resolver landed on a Person.
type resolveOutcome struct {
	PersonID               uuid.UUID
	MatchedViaDeterminator bool
	PersonWasCreated       bool
}

// resolver runs the determinator + upstream + create-if-missing
// algorithm against the supplied mappings for one record's payload.
type resolver struct {
	ctx      context.Context
	tx       bun.IDB
	mappings []*employee_provider_mappings.Mapping
	lookup   persons.AttributeLookup
	now      time.Time
}

// hasPrimary reports whether the provider's mappings authorise it to
// CREATE new Persons (any is_determinator=TRUE).
func (r *resolver) hasPrimary() bool {
	for _, m := range r.mappings {
		if m.IsDeterminator {
			return true
		}
	}
	return false
}

// resolve walks the algorithm. ok=true means a Person was found or
// created; ok=false means the record stays unresolved (the action
// bumps Unresolved counter).
func (r *resolver) resolve(payload map[string]any) (resolveOutcome, bool, error) {
	hasPrimary := r.hasPrimary()

	// Phase 1: direct determinator lookup (every determinator mapping)
	if hasPrimary {
		for _, m := range r.mappings {
			if !m.IsDeterminator {
				continue
			}
			value, _ := payload[m.RecordKey].(string)
			if value == "" {
				continue
			}
			pid, found, err := r.lookup.FindByAttribute(r.ctx, r.tx, m.PersonKey, value)
			if err != nil {
				return resolveOutcome{}, false, err
			}
			if found {
				return resolveOutcome{PersonID: pid, MatchedViaDeterminator: true}, true, nil
			}
		}
		// Phase 1b: no determinator hit — create a Person from the first
		// determinator-keyed value we have.
		for _, m := range r.mappings {
			if !m.IsDeterminator {
				continue
			}
			value, _ := payload[m.RecordKey].(string)
			if value == "" {
				continue
			}
			pid, err := r.createPerson(payload, m.PersonKey, value)
			if err != nil {
				return resolveOutcome{}, false, err
			}
			return resolveOutcome{
				PersonID:               pid,
				MatchedViaDeterminator: true,
				PersonWasCreated:       true,
			}, true, nil
		}
		// hasPrimary but every determinator-keyed value is empty — caller
		// will mark the record unresolved.
		return resolveOutcome{}, false, nil
	}

	// Phase 2: upstream — secondary provider, attaches only.
	for _, m := range r.mappings {
		if !m.AllowUpstream {
			continue
		}
		value, _ := payload[m.RecordKey].(string)
		if value == "" {
			continue
		}
		pid, found, err := r.lookup.FindByAttribute(r.ctx, r.tx, m.PersonKey, value)
		if err != nil {
			return resolveOutcome{}, false, err
		}
		if found {
			return resolveOutcome{PersonID: pid, MatchedViaDeterminator: false}, true, nil
		}
	}
	return resolveOutcome{}, false, nil
}

// createPerson inserts a new Person + its initial determinator
// attribute. external_id is set to the determinator value (it's the
// strongest stable identifier we have at this point); full_name is
// pulled from common payload fields if present.
func (r *resolver) createPerson(payload map[string]any, determinatorKey, determinatorValue string) (uuid.UUID, error) {
	p := &persons.Person{
		ID:         uuid.New(),
		ExternalID: determinatorValue,
		FullName:   pickFullName(payload),
		CreatedAt:  r.now,
		UpdatedAt:  r.now,
	}
	if _, err := r.tx.NewInsert().Model(p).Exec(r.ctx); err != nil {
		return uuid.Nil, err
	}
	if err := upsertPersonAttribute(r.ctx, r.tx, p.ID, determinatorKey, determinatorValue); err != nil {
		return uuid.Nil, err
	}
	return p.ID, nil
}

// upsertPersonAttribute inserts a (person_id, key, value) row, or
// updates the value if the (person_id, key) row already exists.
func upsertPersonAttribute(ctx context.Context, tx bun.IDB, personID uuid.UUID, key, value string) error {
	attr := &persons.PersonAttribute{
		ID:       uuid.New(),
		PersonID: personID,
		Key:      key,
		Value:    value,
	}
	_, err := tx.NewInsert().
		Model(attr).
		On("CONFLICT (person_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

// pickFullName grabs whichever common name field the payload happens
// to carry. Empty string is fine — Person.FullName is NOT NULL but
// has no length minimum.
func pickFullName(payload map[string]any) string {
	for _, k := range []string{"full_name", "name", "display_name"} {
		if v, ok := payload[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

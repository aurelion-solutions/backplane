// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package persons owns the Person entity — a reusable human profile
// root identified by an external system identifier. Employees and
// other principal kinds attach to Persons via foreign key; Person on
// its own carries only the external identity and a free-form set of
// key/value attributes. Persistence is bun on Postgres.
package persons

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Person is the persistent profile root for a human.
type Person struct {
	bun.BaseModel `bun:"table:persons,alias:p"`

	ID         uuid.UUID `bun:"id,pk,type:uuid"        json:"id"`
	ExternalID string    `bun:"external_id,notnull"    json:"external_id"`
	FullName   string    `bun:"full_name,notnull"      json:"full_name"`
	CreatedAt  time.Time `bun:"created_at,notnull"     json:"created_at"`
	UpdatedAt  time.Time `bun:"updated_at,notnull"     json:"updated_at"`
}

// PersonAttribute is a single (key, value) tag attached to a Person.
// (person_id, key) is unique.
type PersonAttribute struct {
	bun.BaseModel `bun:"table:person_attributes,alias:pa"`

	ID       uuid.UUID `bun:"id,pk,type:uuid"     json:"id"`
	PersonID uuid.UUID `bun:"person_id,notnull"   json:"person_id"`
	Key      string    `bun:"key,notnull"         json:"key"`
	Value    string    `bun:"value,notnull"       json:"value"`
}

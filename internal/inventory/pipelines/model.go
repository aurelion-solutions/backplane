// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package pipelines

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Pipeline is the persistent record of one cartridge-defined pipeline
// definition.
//
// Natural key: (CartridgeRef, Name). Soft-deleted rows keep that
// uniqueness so a later resurrection of the same pipeline updates the
// existing row in place.
//
// ContentHash is the loader-computed sha256 over the canonical-sorted
// definition body. The sync loop compares hashes to decide whether a
// row needs an Upsert.
type Pipeline struct {
	bun.BaseModel `bun:"table:pipelines,alias:pip"`

	ID           uuid.UUID      `bun:"id,pk,type:uuid"           json:"id"`
	CartridgeRef string         `bun:"cartridge_ref,notnull"     json:"cartridge_ref"`
	Name         string         `bun:"name,notnull"              json:"name"`
	Version      int            `bun:"version,notnull"           json:"version"`
	ContentHash  string         `bun:"content_hash,notnull"      json:"content_hash"`
	SourcePath   string         `bun:"source_path,notnull"       json:"source_path"`
	IsActive     bool           `bun:"is_active,notnull"         json:"is_active"`
	RemovedAt    *time.Time     `bun:"removed_at"                json:"removed_at,omitempty"`
	Meta         map[string]any `bun:"meta,type:jsonb,notnull"   json:"meta"`
	CreatedAt    time.Time      `bun:"created_at,notnull"        json:"created_at"`
	UpdatedAt    time.Time      `bun:"updated_at,notnull"        json:"updated_at"`
}

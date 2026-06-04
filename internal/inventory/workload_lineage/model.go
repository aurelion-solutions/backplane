// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workload_lineage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Terminus constants classify the resolved end of an ownership chain.
const (
	// TerminusActiveHuman: the chain ends at a human whose at least one
	// employment is active as of the resolve time.
	TerminusActiveHuman = "active_human"
	// TerminusTerminatedHuman: the chain ends at a human all of whose
	// employments have ended.
	TerminusTerminatedHuman = "terminated_human"
	// TerminusUnowned: the workload has no owner_employment_id set.
	TerminusUnowned = "unowned"
	// TerminusBrokenLink: an owner reference exists but the referenced
	// row could not be found (data integrity gap).
	TerminusBrokenLink = "broken_link"
)

// ChainLink is one step in an ownership chain.
type ChainLink struct {
	Kind       string     `json:"kind"` // "workload" | "employment" | "person"
	RefID      string     `json:"ref_id"`
	Label      string     `json:"label,omitempty"`
	Terminated bool       `json:"terminated"`
	EndDate    *time.Time `json:"end_date,omitempty"`
	// Title, StartDate and OrgUnit are populated for the "employment"
	// link so the UI can render the role period instead of an opaque id.
	Title     string     `json:"title,omitempty"`
	StartDate *time.Time `json:"start_date,omitempty"`
	OrgUnit   string     `json:"org_unit,omitempty"`
}

// OwnershipChain is the resolved ownership chain for one workload.
type OwnershipChain struct {
	WorkloadID uuid.UUID   `json:"workload_id"`
	Links      []ChainLink `json:"links"`
	Terminus   string      `json:"terminus"`
	ResolvedAt time.Time   `json:"resolved_at"`
}

// ChainHash returns a deterministic hex-SHA256 over the chain identity
// excluding ResolvedAt (wall-clock would defeat idempotency).
//
// Hash input: workload_id | for each link: kind|ref_id|terminated|end_date | terminus.
// end_date is encoded as RFC3339 if present, "" if absent.
func (c OwnershipChain) ChainHash() string {
	type hashable struct {
		WorkloadID uuid.UUID   `json:"workload_id"`
		Links      []ChainLink `json:"links"`
		Terminus   string      `json:"terminus"`
	}
	// Strip ResolvedAt from links for hashing by normalising end_date text.
	type linkHash struct {
		Kind       string `json:"kind"`
		RefID      string `json:"ref_id"`
		Terminated bool   `json:"terminated"`
		EndDate    string `json:"end_date"`
	}
	links := make([]linkHash, len(c.Links))
	for i, l := range c.Links {
		ed := ""
		if l.EndDate != nil {
			ed = l.EndDate.UTC().Format(time.RFC3339)
		}
		links[i] = linkHash{
			Kind:       l.Kind,
			RefID:      l.RefID,
			Terminated: l.Terminated,
			EndDate:    ed,
		}
	}
	type hInput struct {
		WorkloadID string     `json:"workload_id"`
		Links      []linkHash `json:"links"`
		Terminus   string     `json:"terminus"`
	}
	h := hInput{WorkloadID: c.WorkloadID.String(), Links: links, Terminus: c.Terminus}
	b, _ := json.Marshal(h)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// WorkloadLineageSnapshot is one append-only snapshot row persisted when
// the assess pass resolves a workload's ownership chain.
type WorkloadLineageSnapshot struct {
	bun.BaseModel `bun:"table:workload_lineage_snapshots,alias:wls"`

	ID         uuid.UUID      `bun:"id,pk,type:uuid"                      json:"id"`
	WorkloadID uuid.UUID      `bun:"workload_id,notnull,type:uuid"         json:"workload_id"`
	ResolvedAt time.Time      `bun:"resolved_at,notnull"                   json:"resolved_at"`
	Terminus   string         `bun:"terminus,notnull"                      json:"terminus"`
	Chain      OwnershipChain `bun:"chain,type:jsonb,notnull"              json:"chain"`
	ChainHash  string         `bun:"chain_hash,notnull"                    json:"chain_hash"`
	CreatedAt  time.Time      `bun:"created_at,notnull"                    json:"created_at"`
}

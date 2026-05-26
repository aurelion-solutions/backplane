// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package evidence_chain

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// NormalizedKind values for EvidenceChain.NormalizedKind. The DB CHECK
// constraint enforces the same closed set.
const (
	NormalizedPerson               = "person"
	NormalizedEmployment           = "employment"
	NormalizedAccount              = "account"
	NormalizedWorkload             = "workload"
	NormalizedSecretPlain          = "secret_plain"
	NormalizedSecretCertificate    = "secret_certificate"
	NormalizedConsentedApplication = "consented_application"
	NormalizedConsentGrant         = "consent_grant"
)

// EvidenceChain is one append-only lineage row. All lineage references
// are nullable — a chain records whichever truth layers exist for the
// outcome it explains. ScanRunID and ChainHash are always set.
type EvidenceChain struct {
	bun.BaseModel `bun:"table:evidence_chains,alias:ec"`

	ID                uuid.UUID  `bun:"id,pk,type:uuid"                  json:"id"`
	ScanRunID         uuid.UUID  `bun:"scan_run_id,notnull,type:uuid"     json:"scan_run_id"`
	FindingID         *uuid.UUID `bun:"finding_id,type:uuid"              json:"finding_id,omitempty"`
	OutcomeID         *uuid.UUID `bun:"outcome_id,type:uuid"              json:"outcome_id,omitempty"`
	IngestBatchID     *uuid.UUID `bun:"ingest_batch_id,type:uuid"         json:"ingest_batch_id,omitempty"`
	RawRowHash        *string    `bun:"raw_row_hash"                      json:"raw_row_hash,omitempty"`
	NormalizedKind    *string    `bun:"normalized_kind"                   json:"normalized_kind,omitempty"`
	NormalizedID      *uuid.UUID `bun:"normalized_id,type:uuid"           json:"normalized_id,omitempty"`
	CapabilityGrantID *uuid.UUID `bun:"capability_grant_id,type:uuid"     json:"capability_grant_id,omitempty"`
	InitiativeID      *uuid.UUID `bun:"initiative_id,type:uuid"           json:"initiative_id,omitempty"`
	PolicyRef         string     `bun:"policy_ref,notnull"                json:"policy_ref"`
	ChainHash         string     `bun:"chain_hash,notnull"                json:"chain_hash"`
	CreatedAt         time.Time  `bun:"created_at,notnull"                json:"created_at"`
}

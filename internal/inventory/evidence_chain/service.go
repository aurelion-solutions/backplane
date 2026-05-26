// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package evidence_chain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Service records evidence chains idempotently.
type Service struct {
	repo Repository
}

// NewService constructs a Service over the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// RecordParams is the input to RecordChain. ScanRunID is required; every
// lineage reference is optional and contributes to the chain hash when
// set.
type RecordParams struct {
	ScanRunID         uuid.UUID
	FindingID         *uuid.UUID
	OutcomeID         *uuid.UUID
	IngestBatchID     *uuid.UUID
	RawRowHash        *string
	NormalizedKind    *string
	NormalizedID      *uuid.UUID
	CapabilityGrantID *uuid.UUID
	InitiativeID      *uuid.UUID
	PolicyRef         string
}

// RecordChain computes the deterministic chain hash and appends the
// row. Idempotent: re-recording the same lineage returns the existing
// row without mutating it.
func (s *Service) RecordChain(ctx context.Context, p RecordParams) (*EvidenceChain, error) {
	hash := ChainHash(p)
	row := &EvidenceChain{
		ID:                uuid.New(),
		ScanRunID:         p.ScanRunID,
		FindingID:         p.FindingID,
		OutcomeID:         p.OutcomeID,
		IngestBatchID:     p.IngestBatchID,
		RawRowHash:        p.RawRowHash,
		NormalizedKind:    p.NormalizedKind,
		NormalizedID:      p.NormalizedID,
		CapabilityGrantID: p.CapabilityGrantID,
		InitiativeID:      p.InitiativeID,
		PolicyRef:         p.PolicyRef,
		ChainHash:         hash,
		CreatedAt:         time.Now().UTC(),
	}
	inserted, err := s.repo.Insert(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("record chain: %w", err)
	}
	if inserted {
		return row, nil
	}
	// Lost the race / re-record: return the canonical existing row.
	return s.repo.GetByChainHash(ctx, hash)
}

// ChainHash is a deterministic SHA-256 over the chain's component
// references plus the policy ref. Same lineage → same hash, independent
// of map/field ordering or the row's own id.
func ChainHash(p RecordParams) string {
	parts := []string{
		"scan_run=" + p.ScanRunID.String(),
		"policy=" + p.PolicyRef,
	}
	if p.FindingID != nil {
		parts = append(parts, "finding="+p.FindingID.String())
	}
	if p.OutcomeID != nil {
		parts = append(parts, "outcome="+p.OutcomeID.String())
	}
	if p.IngestBatchID != nil {
		parts = append(parts, "batch="+p.IngestBatchID.String())
	}
	if p.RawRowHash != nil {
		parts = append(parts, "raw="+*p.RawRowHash)
	}
	if p.NormalizedKind != nil && p.NormalizedID != nil {
		parts = append(parts, "norm="+*p.NormalizedKind+":"+p.NormalizedID.String())
	}
	if p.CapabilityGrantID != nil {
		parts = append(parts, "grant="+p.CapabilityGrantID.String())
	}
	if p.InitiativeID != nil {
		parts = append(parts, "initiative="+p.InitiativeID.String())
	}
	sort.Strings(parts)
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

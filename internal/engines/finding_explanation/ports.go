// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"context"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/inventory/evidence_chain"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
)

// FindingsReader reads the finding being explained. The engine only
// reads — it never writes a finding.
type FindingsReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*findings.Finding, error)
}

// EvidenceReader reads the evidence chain a finding rests on — the
// proof the narrative must stay anchored to.
type EvidenceReader interface {
	ListByFinding(ctx context.Context, findingID uuid.UUID) ([]*evidence_chain.EvidenceChain, error)
}

// GenerateRequest is one model call assembled by this engine: the
// optional named backend, the chat messages, and pass-through params.
type GenerateRequest struct {
	Provider string
	Messages []InferenceMessage
	Params   map[string]any
}

// InferenceMessage mirrors the gateway/llm message shape without
// importing the platform package into the domain contract.
type InferenceMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenerateResult is the assembled output of one model call.
type GenerateResult struct {
	Output     string
	TokensUsed int
	ModelRef   string
}

// InferenceClient is the port to cmd/inference-gateway. The engine
// hands it a request and gets back assembled text; it never knows
// whether the gateway used a local executor, a worker pool, llama-server
// or a cloud provider. The real implementation is an HTTP+SSE client to
// the gateway; tests substitute a fake.
type InferenceClient interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error)
}

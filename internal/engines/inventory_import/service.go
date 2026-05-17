// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_import

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/engines/inventory_ingest"
)

// datasetActions maps dataset_type → (engine, action) pair the
// normalize phase dispatches through the registry. Adding a new
// dataset = adding a row here, no other change in this package.
var datasetActions = map[string]struct{ engine, action string }{
	"employee":            {"inventory_normalize", "employee"},
	"account":             {"inventory_normalize", "account"},
	"orgunit":             {"inventory_normalize", "orgunit"},
	"person":              {"inventory_normalize", "person"},
	"access_grant_record": {"inventory_normalize", "access_grant_record"},
}

// Service runs the ingest → normalize sequence synchronously.
type Service struct {
	deps Deps
}

// Deps wires the service to its collaborators.
type Deps struct {
	Ingest  *inventory_ingest.Service
	Actions *registry.Registry
	DB      *bun.DB
	Log     *slog.Logger
}

// NewService constructs the service. Required fields must be
// non-nil — the composition root owns the wiring.
func NewService(d Deps) (*Service, error) {
	if d.Ingest == nil {
		return nil, fmt.Errorf("inventory_import: Ingest is required")
	}
	if d.Actions == nil {
		return nil, fmt.Errorf("inventory_import: Actions is required")
	}
	if d.DB == nil {
		return nil, fmt.Errorf("inventory_import: DB is required")
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{deps: d}, nil
}

// Process drives one (source, dataset_type, records) batch all the
// way from CSV-shaped records to normalized PG rows.
//
// Sequencing:
//
//  1. inventory_ingest.Process with SkipEvent=true — writes lake
//     plus audit row, suppresses the async MQ trigger.
//  2. Look up the normalize action for dataset_type. Unknown
//     dataset_types are rejected.
//  3. Open one bun transaction and dispatch the action with the
//     freshly-produced batch_id / source / lake_ref. The action
//     runs all of its writes against this transaction; on success
//     the tx commits, on failure the tx rolls back and the whole
//     Process returns the error.
//
// Returns ingest counters + the normalize action's raw Result map.
func (s *Service) Process(ctx context.Context, in HTTPRequest) (HTTPResponse, error) {
	ingestReq := inventory_ingest.Request{
		Source:        in.Source,
		DatasetType:   in.DatasetType,
		CorrelationID: in.CorrelationID,
		Records:       in.Records,
		SkipEvent:     true,
	}
	ingestResult, err := s.deps.Ingest.Process(ctx, ingestReq)
	if err != nil {
		return HTTPResponse{}, err
	}

	pair, ok := datasetActions[in.DatasetType]
	if !ok {
		return HTTPResponse{}, fmt.Errorf("inventory_import: no normalize handler for dataset_type %q", in.DatasetType)
	}

	// Re-importing an identical batch hits inventory_ingest's
	// anti-join and produces an empty write-set — `lake_ref` comes
	// back nil. There is nothing for normalize to read, so skip the
	// dispatch entirely and surface that "everything already on
	// file" as a zero-counter normalize result instead of feeding
	// the action an empty lake_ref it then rejects.
	if ingestResult.LakeRef == nil {
		return HTTPResponse{
			Ingest:    *ingestResult,
			Normalize: map[string]any{"skipped_no_changes": true},
		}, nil
	}

	args := map[string]any{
		"batch_id": ingestResult.BatchID,
		"source":   in.Source,
		"lake_ref": *ingestResult.LakeRef,
	}

	var normalizeResult map[string]any
	err = s.deps.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		actCtx := registry.ActionContext{
			Ctx: ctx,
			Tx:  tx,
			Log: s.deps.Log,
			// PipelineRunID / StepRunID stay zero — this is a
			// synthetic dispatch, not a real pipeline run.
			PipelineRunID: uuid.Nil,
			StepRunID:     uuid.Nil,
		}
		out, err := s.deps.Actions.Dispatch(pair.engine, pair.action, args, actCtx)
		if err != nil {
			return fmt.Errorf("normalize.%s: %w", pair.action, err)
		}
		normalizeResult = out
		return nil
	})
	if err != nil {
		return HTTPResponse{}, err
	}

	return HTTPResponse{
		Ingest:    *ingestResult,
		Normalize: normalizeResult,
	}, nil
}

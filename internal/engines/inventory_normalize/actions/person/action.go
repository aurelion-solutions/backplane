// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package person

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/inventory/persons"
)

// LakeReader narrows storage.Storage to the single method the action
// needs.
type LakeReader interface {
	ReadBatch(ctx context.Context, storageKey string) ([]map[string]any, error)
}

// Deps is the composition-root injection set.
type Deps struct {
	Lake LakeReader
}

// New builds the handler closure with deps bound.
func New(deps Deps) registry.Handler[Args, Result] {
	return func(args Args, ctx registry.ActionContext) (Result, error) {
		if args.LakeRef == "" {
			return Result{}, fmt.Errorf("person: empty lake_ref")
		}
		records, err := deps.Lake.ReadBatch(ctx.Ctx, args.LakeRef)
		if err != nil {
			return Result{}, fmt.Errorf("person: read lake %q: %w", args.LakeRef, err)
		}

		res := Result{Read: len(records)}
		now := time.Now().UTC()
		for _, rec := range records {
			extID, fullName, ok := extractPerson(rec)
			if !ok {
				res.Skipped++
				continue
			}
			p := &persons.Person{
				ID:         uuid.New(),
				ExternalID: extID,
				FullName:   fullName,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			_, err := ctx.Tx.NewInsert().Model(p).
				On("CONFLICT (external_id) DO UPDATE").
				Set("full_name  = EXCLUDED.full_name").
				Set("updated_at = EXCLUDED.updated_at").
				Exec(ctx.Ctx)
			if err != nil {
				return Result{}, fmt.Errorf("person: upsert %q: %w", extID, err)
			}
			res.Upserted++
		}
		return res, nil
	}
}

// extractPerson pulls external_id (top-level, mirrors what
// inventory_ingest wrote) and full_name (from payload).
// Both must be non-empty for the record to be honored.
func extractPerson(rec map[string]any) (string, string, bool) {
	extID, _ := rec["external_id"].(string)
	if extID == "" {
		return "", "", false
	}
	payload, _ := rec["payload"].(map[string]any)
	if payload == nil {
		return "", "", false
	}
	fullName, _ := payload["full_name"].(string)
	if fullName == "" {
		return "", "", false
	}
	return extID, fullName, true
}

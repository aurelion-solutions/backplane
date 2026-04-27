// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orgunit

import (
	"context"
	"fmt"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/inventory/org_units"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
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
			return Result{}, fmt.Errorf("orgunit: empty lake_ref")
		}
		records, err := deps.Lake.ReadBatch(ctx.Ctx, args.LakeRef)
		if err != nil {
			return Result{}, fmt.Errorf("orgunit: read lake %q: %w", args.LakeRef, err)
		}
		res := Result{Read: len(records)}
		now := time.Now().UTC()
		for _, rec := range records {
			payload, _ := rec["payload"].(map[string]any)
			if payload == nil {
				res.Skipped++
				continue
			}
			upserted, skipped, err := walkAndUpsert(ctx.Ctx, ctx.Tx, payload, nil, now)
			if err != nil {
				return Result{}, fmt.Errorf("orgunit: walk subtree: %w", err)
			}
			res.Upserted += upserted
			res.Skipped += skipped
		}
		return res, nil
	}
}

// walkAndUpsert processes one node, recurses into its children, and
// returns (upsertedCount, skippedCount).
//
// The walk is top-down so a child always sees its parent's id
// already in Postgres. parent_id is threaded through recursion.
func walkAndUpsert(
	ctx context.Context, tx bun.IDB,
	node map[string]any, parentID *uuid.UUID,
	now time.Time,
) (int, int, error) {
	identifier, _ := node["identifier"].(string)
	name, _ := node["name"].(string)
	if identifier == "" || name == "" {
		return 0, 1, nil // malformed — skip this node (and its children)
	}
	displayName, _ := node["display_name"].(string)
	isActive := true
	if v, ok := node["is_active"].(bool); ok {
		isActive = v
	}

	u := &org_units.OrgUnit{
		ID:          uuid.New(),
		ExternalID:  identifier,
		Name:        name,
		DisplayName: displayName,
		ParentID:    parentID,
		Description: stringPtr(displayName),
		IsActive:    isActive,
		IsInternal:  false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	// UPSERT by external_id; RETURNING id gives us either the freshly
	// inserted row's id or the existing one's, which we then use as
	// parent_id for children.
	var resolvedID uuid.UUID
	err := tx.NewInsert().
		Model(u).
		On("CONFLICT (external_id) DO UPDATE").
		Set("name         = EXCLUDED.name").
		Set("display_name = EXCLUDED.display_name").
		Set("parent_id    = EXCLUDED.parent_id").
		Set("description  = EXCLUDED.description").
		Set("is_active    = EXCLUDED.is_active").
		Set("is_internal  = EXCLUDED.is_internal").
		Set("updated_at   = EXCLUDED.updated_at").
		Returning("id").
		Scan(ctx, &resolvedID)
	if err != nil {
		return 0, 0, err
	}

	upserted := 1
	skipped := 0
	children, _ := node["children"].([]any)
	for _, c := range children {
		child, ok := c.(map[string]any)
		if !ok {
			skipped++
			continue
		}
		u2, s2, err := walkAndUpsert(ctx, tx, child, &resolvedID, now)
		if err != nil {
			return upserted, skipped, err
		}
		upserted += u2
		skipped += s2
	}
	return upserted, skipped, nil
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

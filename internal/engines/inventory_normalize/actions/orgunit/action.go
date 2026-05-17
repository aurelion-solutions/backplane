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
//
// Records arrive in one of two shapes:
//
//   - Tree shape (legacy AD-style): payload is one node with
//     `{identifier, name, …, children: [...]}`. Recursive walk,
//     parent_id threaded through.
//   - Flat shape (CSV-style): payload is one node with
//     `{external_id, name, parent_external_id?}`. Two-pass: upsert
//     every node with parent_id=NULL, then a second pass resolves
//     `parent_external_id` → parent's id and patches `parent_id`.
//
// Detection is per-record: a payload that carries `external_id` is
// treated as flat; one with `identifier` is treated as tree. A batch
// can mix the two (each record judged independently).
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

		flatPayloads := make([]map[string]any, 0, len(records))
		for _, rec := range records {
			payload, _ := rec["payload"].(map[string]any)
			if payload == nil {
				res.Skipped++
				continue
			}
			if _, ok := payload["external_id"].(string); ok {
				flatPayloads = append(flatPayloads, payload)
				continue
			}
			upserted, skipped, err := walkAndUpsert(ctx.Ctx, ctx.Tx, payload, nil, now)
			if err != nil {
				return Result{}, fmt.Errorf("orgunit: walk subtree: %w", err)
			}
			res.Upserted += upserted
			res.Skipped += skipped
		}

		if len(flatPayloads) > 0 {
			upserted, skipped, err := upsertFlatBatch(ctx.Ctx, ctx.Tx, flatPayloads, now)
			if err != nil {
				return Result{}, fmt.Errorf("orgunit: flat batch: %w", err)
			}
			res.Upserted += upserted
			res.Skipped += skipped
		}

		return res, nil
	}
}

// upsertFlatBatch handles CSV-shaped payloads as a two-pass write:
// first every node with parent_id=NULL, then every node whose
// `parent_external_id` is non-empty gets its parent_id patched.
//
// The lookup map covers rows touched in this batch; a parent that
// only exists in PG from a prior batch is also resolved through a
// SELECT inside the same Tx so cross-batch parents work too.
func upsertFlatBatch(
	ctx context.Context, tx bun.IDB,
	payloads []map[string]any, now time.Time,
) (int, int, error) {
	upserted := 0
	skipped := 0

	idByExternal := make(map[string]uuid.UUID, len(payloads))
	withParent := make([]flatNode, 0, len(payloads))

	for _, p := range payloads {
		extID, _ := p["external_id"].(string)
		name, _ := p["name"].(string)
		if extID == "" || name == "" {
			skipped++
			continue
		}
		displayName, _ := p["display_name"].(string)
		if displayName == "" {
			displayName = name
		}
		isActive := true
		if v, ok := p["is_active"].(bool); ok {
			isActive = v
		}

		u := &org_units.OrgUnit{
			ID:          uuid.New(),
			ExternalID:  extID,
			Name:        name,
			DisplayName: displayName,
			ParentID:    nil,
			Description: stringPtr(displayName),
			IsActive:    isActive,
			IsInternal:  false,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		var resolvedID uuid.UUID
		err := tx.NewInsert().
			Model(u).
			On("CONFLICT (external_id) DO UPDATE").
			Set("name         = EXCLUDED.name").
			Set("display_name = EXCLUDED.display_name").
			Set("description  = EXCLUDED.description").
			Set("is_active    = EXCLUDED.is_active").
			Set("is_internal  = EXCLUDED.is_internal").
			Set("updated_at   = EXCLUDED.updated_at").
			Returning("id").
			Scan(ctx, &resolvedID)
		if err != nil {
			return upserted, skipped, err
		}
		idByExternal[extID] = resolvedID
		upserted++

		if parentExt, ok := p["parent_external_id"].(string); ok && parentExt != "" {
			withParent = append(withParent, flatNode{externalID: extID, parentExternalID: parentExt})
		}
	}

	for _, n := range withParent {
		parentID, ok := idByExternal[n.parentExternalID]
		if !ok {
			// Parent not in this batch — look it up in PG (could
			// have been written by an earlier batch).
			var existing org_units.OrgUnit
			err := tx.NewSelect().Model(&existing).
				Column("id").
				Where("external_id = ?", n.parentExternalID).
				Scan(ctx)
			if err != nil {
				// Unknown parent — leave parent_id NULL, count as
				// skipped-edge but do not fail the whole batch.
				skipped++
				continue
			}
			parentID = existing.ID
		}
		_, err := tx.NewUpdate().Model((*org_units.OrgUnit)(nil)).
			Set("parent_id = ?", parentID).
			Set("updated_at = ?", now).
			Where("external_id = ?", n.externalID).
			Exec(ctx)
		if err != nil {
			return upserted, skipped, err
		}
	}

	return upserted, skipped, nil
}

// flatNode caches one (child, parent) edge that needs a second-pass
// parent_id patch.
type flatNode struct {
	externalID       string
	parentExternalID string
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

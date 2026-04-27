// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_grant_record

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/aurelion-solutions/backplane/internal/inventory/capability_grants"
	"github.com/aurelion-solutions/backplane/internal/inventory/capability_mappings"
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
	Lake     LakeReader
	Accounts accounts.Lookup
	Mappings capability_mappings.Repository
	Grants   capability_grants.Repository
}

// New builds the handler closure with deps bound.
func New(deps Deps) registry.Handler[Args, Result] {
	return func(args Args, ctx registry.ActionContext) (Result, error) {
		if args.LakeRef == "" {
			return Result{}, fmt.Errorf("access_grant_record: empty lake_ref")
		}
		records, err := deps.Lake.ReadBatch(ctx.Ctx, args.LakeRef)
		if err != nil {
			return Result{}, fmt.Errorf("access_grant_record: read lake %q: %w", args.LakeRef, err)
		}
		mappings, err := deps.Mappings.ListActive(ctx.Ctx, ctx.Tx)
		if err != nil {
			return Result{}, fmt.Errorf("access_grant_record: list mappings: %w", err)
		}

		res := Result{Read: len(records)}
		now := time.Now().UTC()
		accountCache := map[accountKey]*accounts.Account{}

		for _, rec := range records {
			parsed, ok := parseRecord(rec)
			if !ok {
				res.Skipped++
				continue
			}
			acc, found, err := resolveAccount(ctx.Ctx, ctx.Tx, deps.Accounts, accountCache, parsed.ApplicationID, parsed.Account)
			if err != nil {
				return Result{}, fmt.Errorf("access_grant_record: account lookup: %w", err)
			}
			if !found {
				res.UnresolvedAcct++
				continue
			}

			matchedAny := false
			for _, m := range mappings {
				grant, projected := project(projectInput{
					GrantExternalID: parsed.ExternalID,
					Resource:        parsed.Resource,
					ResourceKind:    parsed.ResourceKind,
					ActionSlug:      parsed.ActionSlug,
					Account:         acc,
					Mapping:         m,
					Now:             now,
				})
				if !projected {
					continue
				}
				if err := deps.Grants.Upsert(ctx.Ctx, ctx.Tx, grant); err != nil {
					return Result{}, fmt.Errorf("access_grant_record: upsert grant: %w", err)
				}
				matchedAny = true
				res.Upserted++
			}
			if !matchedAny {
				// Record arrived but no mapping covers it. Not an error
				// — admin can add the rule later and replay.
				res.Skipped++
			}
		}
		return res, nil
	}
}

// parsedRecord is the action's view of one lake record after
// validation.
type parsedRecord struct {
	ExternalID    string
	ApplicationID uuid.UUID
	Account       string
	Resource      string
	ResourceKind  string
	ActionSlug    string
}

// parseRecord pulls the required fields from a lake record's payload.
// Returns false if any required field is missing or malformed; the
// caller bumps Skipped.
func parseRecord(rec map[string]any) (parsedRecord, bool) {
	payload, _ := rec["payload"].(map[string]any)
	if payload == nil {
		return parsedRecord{}, false
	}
	extID, _ := rec["external_id"].(string)
	appIDStr, _ := payload["application_id"].(string)
	account, _ := payload["account"].(string)
	resource, _ := payload["resource"].(string)
	resourceKind, _ := payload["resource_kind"].(string)
	actionSlug, _ := payload["action_slug"].(string)

	if extID == "" || appIDStr == "" || account == "" || resource == "" {
		return parsedRecord{}, false
	}
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		return parsedRecord{}, false
	}
	return parsedRecord{
		ExternalID:    extID,
		ApplicationID: appID,
		Account:       account,
		Resource:      resource,
		ResourceKind:  resourceKind,
		ActionSlug:    actionSlug,
	}, true
}

// accountKey indexes the per-batch account-resolution cache so we
// don't re-query Postgres for repeat (application, username) pairs.
type accountKey struct {
	ApplicationID uuid.UUID
	Username      string
}

// resolveAccount looks up an Account through the action's cache.
// Returns (acc, true, nil) on hit, (nil, false, nil) when the account
// does not exist, (nil, false, err) on DB failure.
func resolveAccount(
	ctx context.Context,
	tx bun.IDB,
	lookup accounts.Lookup,
	cache map[accountKey]*accounts.Account,
	applicationID uuid.UUID, username string,
) (*accounts.Account, bool, error) {
	key := accountKey{ApplicationID: applicationID, Username: username}
	if a, hit := cache[key]; hit {
		return a, a != nil, nil
	}
	acc, err := lookup.GetByApplicationAndUsername(ctx, tx, applicationID, username)
	if err != nil {
		if errors.Is(err, accounts.ErrNotFound) {
			cache[key] = nil
			return nil, false, nil
		}
		return nil, false, err
	}
	cache[key] = acc
	return acc, true, nil
}

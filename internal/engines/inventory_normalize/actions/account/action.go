// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package account

import (
	"context"
	"fmt"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/google/uuid"
)

// LakeReader narrows storage.Storage to the single method the action
// needs. Lets tests pass a stub without dragging the full provider
// factory in.
type LakeReader interface {
	ReadBatch(ctx context.Context, storageKey string) ([]map[string]any, error)
}

// Deps is the composition-root injection set.
type Deps struct {
	Lake LakeReader
	Repo accounts.Repository
}

// New builds the handler closure with deps bound.
func New(deps Deps) registry.Handler[Args, Result] {
	return func(args Args, ctx registry.ActionContext) (Result, error) {
		if args.LakeRef == "" {
			return Result{}, fmt.Errorf("account: empty lake_ref")
		}
		records, err := deps.Lake.ReadBatch(ctx.Ctx, args.LakeRef)
		if err != nil {
			return Result{}, fmt.Errorf("account: read lake batch %q: %w", args.LakeRef, err)
		}
		res := Result{Read: len(records)}
		now := time.Now().UTC()
		for _, rec := range records {
			acc, ok := buildAccount(rec, args.Source, now)
			if !ok {
				res.Skipped++
				continue
			}
			if err := deps.Repo.Upsert(ctx.Ctx, ctx.Tx, acc); err != nil {
				return Result{}, fmt.Errorf("account: upsert (%s, %s): %w",
					acc.ApplicationID, acc.Username, err)
			}
			res.Upserted++
		}
		return res, nil
	}
}

// buildAccount shapes one lake record into an Account, or returns
// ok=false if the payload is invalid (missing application_id /
// username). External_id is taken from the top-level lake field;
// payload-level external_id is ignored to avoid divergence.
func buildAccount(rec map[string]any, source string, now time.Time) (*accounts.Account, bool) {
	payload, _ := rec["payload"].(map[string]any)
	if payload == nil {
		return nil, false
	}
	appIDStr, _ := payload["application_id"].(string)
	username, _ := payload["username"].(string)
	if appIDStr == "" || username == "" {
		return nil, false
	}
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		return nil, false
	}
	extID, _ := rec["external_id"].(string)

	acc := &accounts.Account{
		ID:            uuid.New(),
		ApplicationID: appID,
		Username:      username,
		ExternalID:    extID,
		Source:        source,
		IsActive:      true,
		IsPrivileged:  false,
		MFAEnabled:    false,
		Attrs:         map[string]any{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if s, ok := payload["display_name"].(string); ok && s != "" {
		acc.DisplayName = &s
	}
	if s, ok := payload["email"].(string); ok && s != "" {
		acc.Email = &s
	}
	if b, ok := payload["is_active"].(bool); ok {
		acc.IsActive = b
	}
	if b, ok := payload["is_privileged"].(bool); ok {
		acc.IsPrivileged = b
	}
	if b, ok := payload["mfa_enabled"].(bool); ok {
		acc.MFAEnabled = b
	}
	if s, ok := payload["status"].(string); ok && s != "" {
		acc.Status = &s
	}
	if a, ok := payload["attrs"].(map[string]any); ok {
		acc.Attrs = a
	}
	acc.EffectiveState = deriveEffectiveState(acc)
	return acc, true
}

// deriveEffectiveState maps the connector-supplied is_active /
// status into the canonical state vocabulary. Order:
//
//  1. If `status` is one of the canonical values, use it verbatim —
//     the connector knows the difference between "invited" and
//     "active" that the boolean is_active can't carry.
//  2. Otherwise fall back to is_active: true → active, false →
//     blocked (an existing-but-disabled account is "blocked", not
//     "not_exist").
//
// `not_exist` is never written by this path — discovery only sees
// accounts the connector reports, which means they exist.
func deriveEffectiveState(a *accounts.Account) string {
	if a.Status != nil {
		switch *a.Status {
		case accounts.StateActive, accounts.StateBlocked, accounts.StateInvited, accounts.StatePending:
			return *a.Status
		}
	}
	if a.IsActive {
		return accounts.StateActive
	}
	return accounts.StateBlocked
}

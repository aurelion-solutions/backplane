// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policies

import (
	"context"
	"log/slog"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/inventory/policies"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// DefaultSyncInterval is the cadence at which the loop wakes up to
// reconcile cartridge state with the PG mirror. Matches the design
// baseline picked in M2 (5 s) — same value used by the consumer-side
// cartridges.Watcher.
const DefaultSyncInterval = 5 * time.Second

// AdvisoryLockKey is the 64-bit integer "AURELPOL" used as the
// per-tick pg_try_advisory_lock key. Lives in shared code so a future
// observability tool can decode it.
const AdvisoryLockKey int64 = 0x4155_5245_4C50_4F4C

// SyncReport summarises what one Sync call did.
type SyncReport struct {
	CartridgesScanned int
	Inserted          int
	Updated           int
	Removed           int
	// Skipped means the advisory lock was held by another replica.
	Skipped bool
}

// Manager owns one Sync iteration.
//
// It holds zero per-call state — every Sync re-reads everything from
// the cartridge provider and the PG mirror. That keeps the recovery
// story trivial (any backplane can take over mid-flight without
// handoff).
type Manager struct {
	provider cartridges.Provider
	repo     policies.Repository
	idGen    func() uuid.UUID
	now      func() time.Time
	log      *slog.Logger
}

// Deps bundles cross-package wiring.
type Deps struct {
	Provider cartridges.Provider
	Repo     policies.Repository
	IDGen    func() uuid.UUID
	Now      func() time.Time
	Log      *slog.Logger
}

// New constructs a Manager.
func New(d Deps) *Manager {
	if d.IDGen == nil {
		d.IDGen = uuid.New
	}
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Manager{
		provider: d.Provider,
		repo:     d.Repo,
		idGen:    d.IDGen,
		now:      d.Now,
		log:      d.Log,
	}
}

// Sync walks every cartridge once and reconciles the policies table.
//
// Algorithm per cartridge:
//
//	manifests = provider.Policies(ref)
//	active    = repo.ListActiveByCartridge(ref)
//	for ruleID, m in manifests:
//	    Upsert(buildPolicy(ref, m))         // insert OR update OR resurrect
//	for ruleID, row in active where ruleID not in manifests:
//	    MarkRemoved(row.id, now)
//
// The Upsert path covers all three transitions in one statement —
// `ON CONFLICT … DO UPDATE` sets is_active=TRUE and clears removed_at,
// which is exactly what resurrection needs.
func (m *Manager) Sync(ctx context.Context) (SyncReport, error) {
	report := SyncReport{}
	refs, err := m.provider.List()
	if err != nil {
		return report, err
	}
	report.CartridgesScanned = len(refs)
	now := m.now()

	for _, ref := range refs {
		manifests, err := m.provider.Policies(ref)
		if err != nil {
			m.log.Warn("policies sync: provider failed",
				slog.String("cartridge", ref.ID), slog.Any("err", err))
			continue
		}
		active, err := m.repo.ListActiveByCartridge(ctx, ref.ID)
		if err != nil {
			m.log.Warn("policies sync: list active failed",
				slog.String("cartridge", ref.ID), slog.Any("err", err))
			continue
		}
		seenInManifest := map[string]struct{}{}
		for ruleID, manifest := range manifests {
			seenInManifest[ruleID] = struct{}{}
			existing, _ := m.repo.GetByNaturalKey(ctx, ref.ID, ruleID)
			row := m.buildPolicy(ref.ID, manifest, existing, now)
			if err := m.repo.Upsert(ctx, row); err != nil {
				m.log.Warn("policies sync: upsert failed",
					slog.String("cartridge", ref.ID),
					slog.String("rule_id", ruleID),
					slog.Any("err", err))
				continue
			}
			if existing == nil {
				report.Inserted++
			} else {
				report.Updated++
			}
		}
		for _, row := range active {
			if _, ok := seenInManifest[row.RuleID]; ok {
				continue
			}
			if err := m.repo.MarkRemoved(ctx, row.ID, now); err != nil {
				m.log.Warn("policies sync: mark removed failed",
					slog.String("cartridge", ref.ID),
					slog.String("rule_id", row.RuleID),
					slog.Any("err", err))
				continue
			}
			report.Removed++
		}
	}
	return report, nil
}

func (m *Manager) buildPolicy(cartridgeRef string, manifest cartridges.Manifest, existing *policies.Policy, now time.Time) *policies.Policy {
	tags := append([]string{}, manifest.Tags...)
	if tags == nil {
		tags = []string{}
	}
	row := &policies.Policy{
		CartridgeRef: cartridgeRef,
		RuleID:       manifest.RuleID,
		Name:         manifest.Name,
		Mechanism:    manifest.Mechanism,
		Tags:         tags,
		Version:      manifest.Version,
		IsActive:     true,
		Meta:         buildMeta(manifest),
		UpdatedAt:    now,
	}
	if manifest.Description != "" {
		desc := manifest.Description
		row.Description = &desc
	}
	if manifest.Severity != "" {
		sev := manifest.Severity
		row.Severity = &sev
	}
	if manifest.OwnerTeam != "" {
		owner := manifest.OwnerTeam
		row.OwnerTeam = &owner
	}
	if existing != nil {
		row.ID = existing.ID
		row.CreatedAt = existing.CreatedAt
	} else {
		row.ID = m.idGen()
		row.CreatedAt = now
	}
	return row
}

// buildMeta packs the open-ended manifest body — mechanism-specific
// payload the platform doesn't interpret — into the meta JSONB column.
// Returns an empty map (not nil) so the column never carries SQL NULL.
func buildMeta(m cartridges.Manifest) map[string]any {
	meta := map[string]any{}
	for k, v := range m.Body {
		meta[k] = v
	}
	return meta
}

// RunSyncLoop ticks every interval and runs Sync under a Postgres
// advisory lock so only one replica reconciles at a time. Other
// replicas tick but observe Skipped=true and do nothing.
//
// The function returns when ctx is cancelled. Tick failures are
// logged and the loop continues.
func (m *Manager) RunSyncLoop(ctx context.Context, db *bun.DB, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultSyncInterval
	}
	m.log.Info("policies sync loop starting", slog.Duration("interval", interval))
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.log.Info("policies sync loop stopping")
			return nil
		case <-t.C:
		}
		if report, err := m.tickWithLock(ctx, db); err != nil {
			m.log.Warn("policies sync tick failed", slog.Any("err", err))
		} else if !report.Skipped && (report.Inserted+report.Updated+report.Removed) > 0 {
			m.log.Info("policies sync tick",
				slog.Int("inserted", report.Inserted),
				slog.Int("updated", report.Updated),
				slog.Int("removed", report.Removed),
				slog.Int("cartridges", report.CartridgesScanned))
		}
	}
}

func (m *Manager) tickWithLock(ctx context.Context, db *bun.DB) (SyncReport, error) {
	var report SyncReport
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var acquired bool
		row := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(?)", AdvisoryLockKey)
		if err := row.Scan(&acquired); err != nil {
			return err
		}
		if !acquired {
			report.Skipped = true
			return nil
		}
		defer func() {
			_, _ = tx.ExecContext(ctx, "SELECT pg_advisory_unlock(?)", AdvisoryLockKey)
		}()
		r, err := m.Sync(ctx)
		report = r
		return err
	})
	return report, err
}

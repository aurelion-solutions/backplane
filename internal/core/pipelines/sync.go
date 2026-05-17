// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package pipelines

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/inventory/pipelines"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// DefaultSyncInterval mirrors core/policies' baseline — 5 s polling.
const DefaultSyncInterval = 5 * time.Second

// AdvisoryLockKey is the 64-bit integer "AURELPIP" used as the
// per-tick pg_try_advisory_lock key.
const AdvisoryLockKey int64 = 0x4155_5245_4C50_4950

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
type Manager struct {
	provider cartridges.Provider
	loader   *loader.Loader
	repo     pipelines.Repository
	idGen    func() uuid.UUID
	now      func() time.Time
	log      *slog.Logger
}

// Deps bundles cross-package wiring.
type Deps struct {
	Provider cartridges.Provider
	Loader   *loader.Loader
	Repo     pipelines.Repository
	IDGen    func() uuid.UUID
	Now      func() time.Time
	Log      *slog.Logger
}

// New constructs a Manager. The loader is required — the sync uses
// its content-hash to decide whether a row needs an Upsert.
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
	if d.Loader == nil {
		d.Loader = loader.New()
	}
	return &Manager{
		provider: d.Provider,
		loader:   d.Loader,
		repo:     d.Repo,
		idGen:    d.IDGen,
		now:      d.Now,
		log:      d.Log,
	}
}

// Sync walks every cartridge once and reconciles the pipelines table.
//
// Algorithm per cartridge:
//
//	paths     = provider.Pipelines(ref)
//	active    = repo.ListActiveByCartridge(ref)
//	for path in paths:
//	    defn = loader.LoadFile(path)
//	    Upsert(buildPipeline(ref, defn))    // insert OR update OR resurrect
//	for name, row in active where name not in defs:
//	    MarkRemoved(row.id, now)
//
// A YAML that fails to load is logged and skipped — corrupt files do
// not knock out the rest of the cartridge.
func (m *Manager) Sync(ctx context.Context) (SyncReport, error) {
	report := SyncReport{}
	refs, err := m.provider.List()
	if err != nil {
		return report, err
	}
	report.CartridgesScanned = len(refs)
	now := m.now()

	for _, ref := range refs {
		paths, err := m.provider.Pipelines(ref)
		if err != nil {
			m.log.Warn("pipelines sync: provider failed",
				slog.String("cartridge", ref.ID), slog.Any("err", err))
			continue
		}
		active, err := m.repo.ListActiveByCartridge(ctx, ref.ID)
		if err != nil {
			m.log.Warn("pipelines sync: list active failed",
				slog.String("cartridge", ref.ID), slog.Any("err", err))
			continue
		}
		seen := map[string]struct{}{}
		for _, path := range paths {
			defn, err := m.loader.LoadFile(path)
			if err != nil {
				m.log.Warn("pipelines sync: load failed",
					slog.String("cartridge", ref.ID),
					slog.String("path", filepath.Base(path)),
					slog.Any("err", err))
				continue
			}
			seen[defn.Name] = struct{}{}
			existing, _ := m.repo.GetByNaturalKey(ctx, ref.ID, defn.Name)
			row := m.buildPipeline(ref.ID, defn, existing, now)
			if err := m.repo.Upsert(ctx, row); err != nil {
				m.log.Warn("pipelines sync: upsert failed",
					slog.String("cartridge", ref.ID),
					slog.String("name", defn.Name),
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
			if _, ok := seen[row.Name]; ok {
				continue
			}
			if err := m.repo.MarkRemoved(ctx, row.ID, now); err != nil {
				m.log.Warn("pipelines sync: mark removed failed",
					slog.String("cartridge", ref.ID),
					slog.String("name", row.Name),
					slog.Any("err", err))
				continue
			}
			report.Removed++
		}
	}
	return report, nil
}

func (m *Manager) buildPipeline(cartridgeRef string, defn *loader.Definition, existing *pipelines.Pipeline, now time.Time) *pipelines.Pipeline {
	row := &pipelines.Pipeline{
		CartridgeRef: cartridgeRef,
		Name:         defn.Name,
		Version:      defn.Version,
		ContentHash:  defn.ContentHash,
		SourcePath:   defn.SourcePath,
		IsActive:     true,
		Meta:         map[string]any{},
		UpdatedAt:    now,
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

// RunSyncLoop ticks every interval and runs Sync under a Postgres
// advisory lock so only one replica reconciles at a time.
func (m *Manager) RunSyncLoop(ctx context.Context, db *bun.DB, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultSyncInterval
	}
	m.log.Info("pipelines sync loop starting", slog.Duration("interval", interval))
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.log.Info("pipelines sync loop stopping")
			return nil
		case <-t.C:
		}
		if report, err := m.tickWithLock(ctx, db); err != nil {
			m.log.Warn("pipelines sync tick failed", slog.Any("err", err))
		} else if !report.Skipped && (report.Inserted+report.Updated+report.Removed) > 0 {
			m.log.Info("pipelines sync tick",
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

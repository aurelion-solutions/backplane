// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
)

// RunCatalogWatcher polls the cartridges filesystem for changes and
// rebuilds the pipeline catalog on every diff. Blocks until ctx is
// cancelled.
//
// Run this in its own goroutine. A reload failure leaves the previous
// catalogue in effect — the runner keeps executing last-good
// definitions until the next tick recovers.
//
// `root` must be the directory the cartridges Provider reads from on
// disk; the watcher walks it directly for mtime polling. interval
// defaults to cartridges.DefaultPollInterval when <= 0.
func RunCatalogWatcher(
	ctx context.Context,
	catalog *Catalog,
	provider cartridges.Provider,
	pipelineLoader *loader.Loader,
	cartridgeIDs []string,
	root string,
	interval time.Duration,
	log *slog.Logger,
) {
	if interval <= 0 {
		interval = cartridges.DefaultPollInterval
	}
	w := cartridges.NewWatcher(
		root,
		cartridges.WatchSuffixes(".yaml"),
		cartridges.WatchLogger(log),
	)
	_ = w.Run(ctx, interval, func(ctx context.Context) error {
		if err := catalog.Reload(provider, pipelineLoader, cartridgeIDs); err != nil {
			return err
		}
		log.Info("pipeline catalog reloaded",
			slog.Int("pipelines", len(catalog.All())),
			slog.Any("cartridges", catalog.Sources()))
		return nil
	})
}

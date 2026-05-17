// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment

import (
	"context"
	"log/slog"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

// RunStoreWatcher polls the cartridges root for changes (mtime poll)
// and rebuilds the in-memory Store on every diff. Blocks until ctx is
// cancelled.
//
// Run this in its own goroutine. A reload failure leaves the previous
// catalogue in effect — the engine keeps serving last-good entries
// until the next tick recovers.
//
// `root` must be the directory the cartridges Provider reads from on
// disk; the watcher walks it directly for mtime polling. interval
// defaults to cartridges.DefaultPollInterval when <= 0.
func RunStoreWatcher(
	ctx context.Context,
	store *Store,
	provider cartridges.Provider,
	root string,
	interval time.Duration,
	log *slog.Logger,
) {
	if interval <= 0 {
		interval = cartridges.DefaultPollInterval
	}
	w := cartridges.NewWatcher(
		root,
		cartridges.WatchSuffixes(".meta.json", ".cedar", ".prompt"),
		cartridges.WatchLogger(log),
	)
	_ = w.Run(ctx, interval, func(ctx context.Context) error {
		n, err := store.Reload(ctx, provider)
		if err != nil {
			return err
		}
		log.Info("policy_assessment store reloaded", slog.Int("entries", n))
		return nil
	})
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command pdp is the Policy Decision Point — the request/response
// process for any short-lived, low-latency identity-related
// evaluation. Today: skeleton. The real evaluation surface lands when
// the cedar mechanism handler (engines/policy_assessment/mechanisms/
// cedar) is wired in.
//
// The PDP is the SLO-isolated home for AuthZ (~10 ms p99) and the
// secondary home for AuthN (~5 s p99). It is *not* the home for batch
// / scan mechanisms — those live in cmd/worker.
//
// On boot the skeleton:
//  1. Loads config via the same secret-manager plumbing as the other
//     binaries (AURELION_SECRET_PROVIDER, AURELION_SECRETS_FILE).
//     Refuses to start without AURELION_PDP_INSTANCE_ID.
//  2. Connects to Postgres and RabbitMQ (so the future cedar handler
//     can load snapshots and subscribe to invalidations without
//     re-wiring).
//  3. Resolves the cartridges provider so future mechanism handlers
//     can load policies the same way every other consumer does.
//  4. Serves HTTP on :8100 — only /healthz today.
//
// HTTP surface (will grow):
//   - GET /healthz — instance metadata + cartridges root.
//   - POST /access/v1/evaluation — AuthZen evaluation (not yet
//     wired; will land with the cedar handler).
//
// Required env:
//   - AURELION_PDP_INSTANCE_ID — unique per replica.
//
// Optional env (defaults shown):
//   - AURELION_PDP_HTTP_ADDR (:8100) — HTTP bind address.
//   - AURELION_SECRET_PROVIDER (file)
//   - AURELION_SECRETS_FILE (.secrets.json)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aurelion-solutions/backplane/cmd/pdp/transport/authzen"
	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/postgres"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/core/webserver"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	cedarmech "github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/mechanisms/cedar"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
)

const (
	logLevel        = "info"
	defaultHTTPAddr = ":8100"
)

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("pdp failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("pdp — Policy Decision Point (skeleton)")
	fmt.Println()
	fmt.Println("  The AuthZ / AuthN host process. Mechanism handlers")
	fmt.Println("  (cedar, future llm-driven mechanisms) wire in here.")
	fmt.Println()
	fmt.Println("  Required: AURELION_PDP_INSTANCE_ID")
	fmt.Println()
}

func run(log *slog.Logger) error {
	_ = godotenv.Load()

	instanceID := strings.TrimSpace(os.Getenv("AURELION_PDP_INSTANCE_ID"))
	if instanceID == "" {
		return errors.New("AURELION_PDP_INSTANCE_ID is required")
	}
	log = log.With(slog.String("instance_id", instanceID))

	providerName := envOr("AURELION_SECRET_PROVIDER", "file")
	secretsFile := envOr("AURELION_SECRETS_FILE", ".secrets.json")

	sf := secretmanagers.NewFactory()
	secretmanagers.RegisterFile(sf, secretsFile)
	secretmanagers.RegisterVault(sf)
	secretmanagers.RegisterOpenBao(sf)
	secretmanagers.RegisterAkeyless(sf)
	secretmanagers.RegisterConjur(sf)

	sm, err := sf.Get(providerName)
	if err != nil {
		return fmt.Errorf("secret provider: %w", err)
	}
	settings, err := config.Load(sm)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := postgres.New(ctx, postgres.Config{DSN: settings.Postgres.DSN(), Debug: settings.App.Debug})
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer func() { _ = db.Close() }()
	log.Info("postgres connected")

	mq, err := rabbitmq.New(rabbitmq.Config{
		URL: settings.RabbitMQ.URL(),
		Exchanges: []rabbitmq.Exchange{
			{Name: settings.RabbitMQ.EventsExchange, Type: rabbitmq.Topic},
		},
	})
	if err != nil {
		return fmt.Errorf("rabbitmq: %w", err)
	}
	defer func() { _ = mq.Close() }()
	log.Info("rabbitmq connected")

	cf := cartridges.NewFactory()
	cartridges.RegisterFilesystem(cf, settings.Cartridges.Root)
	cartridgesProvider, err := cf.Get(settings.Cartridges.Provider)
	if err != nil {
		return fmt.Errorf("cartridges provider: %w", err)
	}
	log.Info("cartridges provider selected",
		slog.String("provider", settings.Cartridges.Provider),
		slog.String("root", settings.Cartridges.Root),
	)

	// Engine wiring: dispatcher + cedar mechanism handler.
	dispatcher := policy_assessment.NewDispatcher()
	cedarHandler := cedarmech.New()
	dispatcher.Register(cedarHandler)
	log.Info("dispatcher registered handlers",
		slog.Any("mechanisms", dispatcher.Mechanisms()))

	// Policy store — in-memory catalogue rebuilt from cartridges. Boot
	// load is best-effort; reload retries every watcher tick.
	store := policy_assessment.NewStore()
	if n, err := store.Reload(ctx, cartridgesProvider); err != nil {
		log.Warn("initial policy store load failed", slog.Any("err", err))
	} else {
		log.Info("policy store loaded", slog.Int("entries", n))
		if okN, errs := dispatcher.PrepareAll(ctx, store.All()); len(errs) > 0 {
			log.Warn("initial handler prepare failures",
				slog.Int("ok", okN), slog.Int("failed", len(errs)))
			for _, e := range errs {
				log.Warn("prepare", slog.Any("err", e))
			}
		} else {
			log.Info("handlers prepared", slog.Int("ok", okN))
		}
	}

	// Watcher — rebuild store + re-prepare on cartridge change.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		policy_assessment.RunStoreWatcher(
			ctx,
			store,
			cartridgesProvider,
			settings.Cartridges.Root,
			cartridges.DefaultPollInterval,
			log.With(slog.String("component", "policy_store_watcher")),
		)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		runPreparePoller(ctx, store, dispatcher, cartridges.DefaultPollInterval, log)
	}()

	addr := envOr("AURELION_PDP_HTTP_ADDR", defaultHTTPAddr)
	e := webserver.New(webserver.Config{
		Debug:            settings.App.Debug,
		CORSAllowOrigins: settings.App.CORSAllowOrigins,
	}, log)
	registerHealthRoute(e, instanceID, settings.Cartridges.Root, store, dispatcher)

	authzen.RegisterRoutes(e.Group(""), authzen.Deps{
		Store:      store,
		Dispatcher: dispatcher,
	})

	serveErr := make(chan error, 1)
	go func() {
		log.Info("http listening", slog.String("addr", addr))
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown error", slog.Any("err", err))
	}
	log.Info("pdp stopped")
	return nil
}

// registerHealthRoute mounts GET /healthz.
func registerHealthRoute(
	e *echo.Echo,
	instanceID string,
	cartridgesRoot string,
	store *policy_assessment.Store,
	dispatcher *policy_assessment.Dispatcher,
) {
	e.GET("/healthz", func(c echo.Context) error {
		entries := store.All()
		mechCounts := map[string]int{}
		for _, e := range entries {
			mechCounts[e.Manifest.Mechanism]++
		}
		return c.JSON(http.StatusOK, map[string]any{
			"status":            "ok",
			"instance_id":       instanceID,
			"role":              "pdp",
			"cartridges_root":   cartridgesRoot,
			"handlers":          dispatcher.Mechanisms(),
			"policies_total":    len(entries),
			"policies_per_mech": mechCounts,
		})
	})
}

// runPreparePoller re-runs Prepare on every store snapshot at a steady
// interval. The watcher already triggers store.Reload on mtime change,
// but preparing inside the watcher would couple two concerns; this
// goroutine just keeps handler caches in sync with whatever the store
// last published.
//
// Side note: Handler.Prepare is idempotent — re-running it with the
// same Entry is cheap (re-read file + re-compile). When we want to
// avoid the recompile cost we'll plumb a content hash into Entry; for
// now this is fine.
func runPreparePoller(
	ctx context.Context,
	store *policy_assessment.Store,
	dispatcher *policy_assessment.Dispatcher,
	interval time.Duration,
	log *slog.Logger,
) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		_, errs := dispatcher.PrepareAll(ctx, store.All())
		for _, err := range errs {
			log.Warn("prepare poll failure", slog.Any("err", err))
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

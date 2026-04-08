// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command worker is a stand-alone orchestrator runner. It claims
// pending Pipeline Runs from Postgres (FOR UPDATE SKIP LOCKED),
// executes their steps via the in-process action registry, and writes
// status back through orchestrator.Service.
//
// Scale-out: run N processes; each opens E executor slots
// (AURELION_WORKER_SLOTS, default 4). Slots compete for the same
// pending queue — at-most-one delivery is enforced by SKIP LOCKED +
// the status-guarded UPDATE inside Service.ClaimPendingRun.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aurelion-solutions/backplane/internal/actions/noop"
	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/runner"
	"github.com/aurelion-solutions/backplane/internal/core/postgres"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/joho/godotenv"
)

const (
	logLevel          = "info"
	defaultWorkerSlots = 4
)

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("worker failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("worker — orchestrator runner node")
	fmt.Println()
	fmt.Println("  Claims pending Pipeline Runs from Postgres (FOR UPDATE SKIP LOCKED)")
	fmt.Println("  and executes their steps in process. Tune slot count via")
	fmt.Println("  AURELION_WORKER_SLOTS (default 4).")
	fmt.Println()
}

func run(log *slog.Logger) error {
	_ = godotenv.Load()

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
		return err
	}
	settings, err := config.Load(sm)
	if err != nil {
		return err
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
			{Name: settings.RabbitMQ.LogsExchange, Type: rabbitmq.Topic},
		},
	})
	if err != nil {
		return err
	}
	defer func() { _ = mq.Close() }()
	log.Info("rabbitmq connected")

	bootSink := siem.NewMQSink(mq.Channel, settings.RabbitMQ.LogsExchange)
	siem.EmitInfo(ctx, bootSink, "worker", "worker started")
	defer siem.EmitInfo(context.Background(), bootSink, "worker", "worker stopping")

	// Cartridges + pipeline catalog.
	cf := cartridges.NewFactory()
	cartridges.RegisterFilesystem(cf, settings.Cartridges.Root)
	provider, err := cf.Get(settings.Cartridges.Provider)
	if err != nil {
		return err
	}
	log.Info("cartridges provider selected",
		slog.String("provider", settings.Cartridges.Provider),
		slog.String("root", settings.Cartridges.Root),
	)

	// Action registry + noop actions (smoke / HITL test surface).
	reg := registry.New()
	noop.Register(reg)
	log.Info("action registry ready", slog.Int("actions", len(reg.All())))

	pipelineLoader := &loader.Loader{Actions: nil} // see backplane main for rationale
	catalog, err := orchestrator.LoadFromCartridges(provider, pipelineLoader, nil)
	if err != nil {
		return fmt.Errorf("orchestrator: load pipelines: %w", err)
	}
	log.Info("pipeline catalog loaded",
		slog.Int("pipelines", len(catalog.All())),
		slog.Any("cartridges", catalog.Sources()),
	)

	svc := orchestrator.NewService(orchestrator.NewBunRepository())

	slots := envInt("AURELION_WORKER_SLOTS", defaultWorkerSlots)
	tags := envTags("AURELION_WORKER_TAGS")
	log.Info("starting worker slots",
		slog.Int("slots", slots),
		slog.Any("tags", tags),
	)

	var wg sync.WaitGroup
	for i := 0; i < slots; i++ {
		wid := runner.NewWorkerIdentity(i, tags)
		r := runner.New(db, svc, reg, catalog, log.With(slog.Int("slot", i)), wid)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.WorkLoop(ctx); err != nil {
				log.Error("work loop terminated", slog.Any("err", err))
			}
		}()
	}

	<-ctx.Done()
	log.Info("shutdown signal received — waiting for slots to drain")

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelDrain()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Info("worker stopped cleanly")
	case <-drainCtx.Done():
		log.Warn("worker drain timeout — exiting (reclaim sweep will pick up stragglers)")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// envTags parses a CSV env var (e.g. "gpu,llm,prod") into a deduped,
// trimmed slice of non-empty entries. Returns nil for unset/empty
// vars — runner.NewWorkerIdentity makes the defensive copy.
func envTags(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range strings.Split(v, ",") {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// Suppress unused-import warning when one of the imports isn't yet
// reached on a particular code path during refactors.
var _ = errors.New

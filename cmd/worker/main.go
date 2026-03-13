// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command worker is a stand-alone orchestrator runner. It claims
// dispatched Steps from the broker and reports results back to the
// backplane API.
//
// Skeleton: boots a Runner with no real executors registered and
// holds the heartbeat loop until SIGINT/SIGTERM. Connects to MQ only
// to emit start/stop lifecycle events.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/engines/orchestrator"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/joho/godotenv"
)

const logLevel = "info"

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
	fmt.Println("  Claims dispatched Pipeline Steps from the broker, runs them via")
	fmt.Println("  registered StepExecutors, and reports results back to the backplane.")
	fmt.Println("  No HTTP, no endpoints — pure worker loop. Scale by running more nodes.")
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

	// Skeleton wiring: no repo, no loader, no dispatcher yet — Service
	// only acts as the inbound callback for ReportStepResult once it
	// gets a real implementation.
	svc := orchestrator.NewService(nil, nil, nil)

	runner := orchestrator.NewRunner(log, svc /* no StepExecutors registered */)
	return runner.Run(ctx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

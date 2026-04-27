// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command ingester is the lake-stream worker. It consumes
// aurelion.ingest one message per record, windows incoming records
// per (source, dataset_type, correlation_id), runs the DuckDB
// anti-join against the current lake state, and writes only
// new/changed records into the lake — then emits
// inventory.ingest.batch_received.
//
// Why a separate process:
//   - backplane stays responsive (no million-message consumer
//     hogging its goroutines),
//   - worker stays a pure orchestrator runner (no shared concerns
//     with lake writes, free to grow external actions),
//   - ingester scales horizontally — N replicas drain the same
//     durable queue, AMQP shards deliveries automatically.
//
// Required env:
//   - AURELION_INGESTER_INSTANCE_ID — unique per replica; surfaced
//     in logs and AMQP consumer tag. The process refuses to boot
//     without it.
//
// Optional env (defaults shown):
//   - AURELION_SECRET_PROVIDER (file) — secret backend selector
//   - AURELION_SECRETS_FILE (.secrets.json) — file-backend path
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/postgres"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/engines/inventory_ingest"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/aurelion-solutions/backplane/internal/platform/storage"
	"github.com/aurelion-solutions/backplane/internal/transports/ingest_mq"
	"github.com/joho/godotenv"
)

const (
	logLevel        = "info"
	storageProvider = "file"
)

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("ingester failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("ingester — lake-stream worker")
	fmt.Println()
	fmt.Println("  Consumes aurelion.ingest (one record per message),")
	fmt.Println("  windows them per (source, dataset_type, correlation_id),")
	fmt.Println("  hashes + DuckDB anti-joins + writes only changed records.")
	fmt.Println()
	fmt.Println("  Required: AURELION_INGESTER_INSTANCE_ID")
	fmt.Println()
}

func run(log *slog.Logger) error {
	_ = godotenv.Load()

	instanceID := os.Getenv("AURELION_INGESTER_INSTANCE_ID")
	if instanceID == "" {
		return errors.New("AURELION_INGESTER_INSTANCE_ID is required")
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
			{Name: settings.RabbitMQ.EventsExchange, Type: rabbitmq.Topic},
			{Name: settings.RabbitMQ.LogsExchange, Type: rabbitmq.Topic},
			{Name: ingest_mq.DefaultExchange, Type: rabbitmq.Topic},
		},
	})
	if err != nil {
		return err
	}
	defer func() { _ = mq.Close() }()
	log.Info("rabbitmq connected")

	bootSink := siem.NewMQSink(mq.Channel, settings.RabbitMQ.LogsExchange)
	siem.EmitInfo(ctx, bootSink, "ingester", "ingester started")
	defer siem.EmitInfo(context.Background(), bootSink, "ingester", "ingester stopping")

	eventsSink := events.NewMQ(mq.Channel, settings.RabbitMQ.EventsExchange)

	stf := storage.NewFactory()
	storage.RegisterFile(stf, storage.DefaultBasePath)
	storage.RegisterS3(stf)
	storage.RegisterIceberg(stf)
	st, err := stf.Get(storageProvider)
	if err != nil {
		return err
	}
	log.Info("storage selected", slog.String("provider", storageProvider))

	ingestRepo := inventory_ingest.NewBunRepository(db)
	ingestSvc := inventory_ingest.NewService(inventory_ingest.Deps{
		Repo: ingestRepo,
		Lake: ingestLakeAdapter{storage: st},
		Sink: eventsSink,
	})

	consumeChan, err := mq.Conn.Channel()
	if err != nil {
		return fmt.Errorf("ingester: open consume channel: %w", err)
	}
	defer func() { _ = consumeChan.Close() }()

	log.Info("ingest_mq starting",
		slog.String("exchange", ingest_mq.DefaultExchange),
		slog.String("queue", ingest_mq.DefaultQueue),
	)

	consumerErr := make(chan error, 1)
	go func() {
		consumerErr <- ingest_mq.Run(ctx, ingest_mq.Config{
			Channel:  consumeChan,
			Exchange: ingest_mq.DefaultExchange,
			Queue:    ingest_mq.DefaultQueue,
			Service:  ingestSvc,
			Log:      log.With(slog.String("component", "ingest_mq")),
		})
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received — draining buffered records")
	case err := <-consumerErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	// Give the consumer up to 30s to flush in-flight buckets.
	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelDrain()
	select {
	case <-consumerErr:
		log.Info("ingester stopped cleanly")
	case <-drainCtx.Done():
		log.Warn("ingester drain timeout — exiting (in-flight records may redeliver)")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ingestLakeAdapter implements inventory_ingest.Lake.
type ingestLakeAdapter struct{ storage storage.Storage }

func (a ingestLakeAdapter) WriteBatch(ctx context.Context, datasetType string, records []map[string]any) (string, error) {
	return a.storage.WriteBatch(ctx, datasetType, records)
}

func (a ingestLakeAdapter) AntiJoin(ctx context.Context, datasetType string, candidates []storage.Candidate) (storage.AntiJoinResult, error) {
	return a.storage.AntiJoin(ctx, datasetType, candidates)
}

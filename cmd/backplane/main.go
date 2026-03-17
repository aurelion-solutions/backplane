// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command backplane is the single composition root for the
// aurelion-backplane service. Wiring order:
//
//	envvars → secret.Factory → secret.Manager → config.Settings →
//	logger → postgres.DB → rabbitmq.Conn → events sink → storage →
//	siem → llm → connector RPC client → registration consumer →
//	webserver → serve.
//
// Each factory fails fast: an unreachable dependency at startup aborts
// the boot with a non-zero exit. Hexagonal-style: domain packages
// receive their infra dependencies through constructor functions
// called from here, not via globals.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/postgres"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/core/webserver"
	"github.com/aurelion-solutions/backplane/internal/integrations/applications"
	"github.com/aurelion-solutions/backplane/internal/integrations/connectors"
	"github.com/aurelion-solutions/backplane/internal/platform/llm"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/aurelion-solutions/backplane/internal/platform/storage"
	"github.com/joho/godotenv"
	"github.com/uptrace/bun"
)

// App-side constants. Everything that is NOT a secret lives here, not
// in env. Env reads are reserved for "where to find the secret store".
const (
	httpAddr        = ":8000"
	logLevel        = "info"
	storageProvider = "file"
	llmProvider     = "llamacpp"
)

// siemProviders are fanned out via siem.MultiSink — every Event goes
// to every listed sink, in order.
var siemProviders = []string{"file", "stdout"}

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("startup failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("backplane — Aurelion API composition root")
	fmt.Println()
	fmt.Printf("  HTTP listening on %s\n", httpAddr)
	fmt.Printf("  curl localhost%s/healthz\n", httpAddr)
	fmt.Printf("  curl localhost%s/api/v0/applications\n", httpAddr)
	fmt.Println()
}

func run(log *slog.Logger) error {
	// Bootstrap env: only the secret-provider selection is read from env.
	// Missing .env is non-fatal — production uses real env vars.
	_ = godotenv.Load()

	// The ONLY env reads in the whole service: how to reach the secret
	// store. Everything downstream comes from there or from in-code
	// constants — never from env.
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
	log.Info("config loaded",
		slog.String("postgres_host", settings.Postgres.Host),
		slog.String("rabbitmq_host", settings.RabbitMQ.Host),
		slog.Bool("debug", settings.App.Debug),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pgCfg := postgres.Config{
		DSN:   settings.Postgres.DSN(),
		Debug: settings.App.Debug,
	}
	var db *bun.DB
	for attempt := 1; ; attempt++ {
		db, err = postgres.New(ctx, pgCfg)
		if err == nil {
			break
		}
		log.Warn("postgres connect failed; retrying",
			slog.Int("attempt", attempt),
			slog.Any("err", err),
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	defer func() { _ = db.Close() }()
	log.Info("postgres connected")

	mqCfg := rabbitmq.Config{
		URL: settings.RabbitMQ.URL(),
		Exchanges: []rabbitmq.Exchange{
			{Name: settings.RabbitMQ.EventsExchange, Type: rabbitmq.Topic},
			{Name: settings.RabbitMQ.LogsExchange, Type: rabbitmq.Topic},
			{Name: settings.RabbitMQ.ConnectorCommandsExchange, Type: rabbitmq.Direct},
			{Name: settings.RabbitMQ.ConnectorResponsesExchange, Type: rabbitmq.Direct},
			{Name: settings.RabbitMQ.ConnectorRegistrationExchange, Type: rabbitmq.Topic},
		},
	}
	var mq *rabbitmq.Conn
	for attempt := 1; ; attempt++ {
		mq, err = rabbitmq.New(mqCfg)
		if err == nil {
			break
		}
		log.Warn("rabbitmq connect failed; retrying",
			slog.Int("attempt", attempt),
			slog.Any("err", err),
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	defer func() { _ = mq.Close() }()
	log.Info("rabbitmq connected")

	bootSink := siem.NewMQSink(mq.Channel, settings.RabbitMQ.LogsExchange)
	siem.EmitInfo(ctx, bootSink, "backplane", "backplane started")
	defer siem.EmitInfo(context.Background(), bootSink, "backplane", "backplane stopping")

	eventsSink := events.NewMQ(mq.Channel, settings.RabbitMQ.EventsExchange)
	log.Info("events sink ready")

	// Storage factory: file is real; s3 / iceberg are stubs.
	stf := storage.NewFactory()
	storage.RegisterFile(stf, storage.DefaultBasePath)
	storage.RegisterS3(stf)
	storage.RegisterIceberg(stf)

	st, err := stf.Get(storageProvider)
	if err != nil {
		return err
	}
	_ = st
	log.Info("storage selected", slog.String("provider", storageProvider))

	// LogSink factory: file + mq are real; the rest are stubs that
	// return ErrNotImplemented from Emit. Replace the corresponding
	// type in internal/platform/siem/ to ship a real backend.
	lsf := siem.NewFactory()
	siem.RegisterFile(lsf, siem.DefaultFilePath)
	siem.RegisterStdout(lsf)
	siem.RegisterMQ(lsf, mq.Channel, settings.RabbitMQ.LogsExchange)
	siem.RegisterELK(lsf)
	siem.RegisterFluentd(lsf)
	siem.RegisterLoki(lsf)
	siem.RegisterNagios(lsf)
	siem.RegisterQRadar(lsf)
	siem.RegisterRsyslog(lsf)
	siem.RegisterSeq(lsf)
	siem.RegisterSplunk(lsf)
	siem.RegisterZabbix(lsf)

	sinks := make([]siem.Sink, 0, len(siemProviders))
	for _, name := range siemProviders {
		s, err := lsf.Get(name)
		if err != nil {
			return err
		}
		sinks = append(sinks, s)
	}
	siemSink := siem.NewMulti(sinks...)
	_ = siemSink
	log.Info("siem selected", slog.Any("providers", siemProviders))

	// LLM factory: every backend is currently a stub. Replace the
	// corresponding type in internal/platform/llm/ with a real impl.
	llf := llm.NewFactory()
	llm.RegisterLlamaCpp(llf)
	llm.RegisterAnthropic(llf)
	llm.RegisterOpenAI(llf)

	llmClient, err := llf.Get(llmProvider)
	if err != nil {
		return err
	}
	_ = llmClient
	log.Info("llm selected", slog.String("provider", llmProvider))

	// Connector RPC client (generic AMQP request/reply on a dedicated
	// channel) + the connector-specific protocol wrapper. Reading large
	// connector responses out of the data lake goes through a small
	// adapter over the storage factory.
	rpc := rabbitmq.NewRPCClient(mq.Conn, rabbitmq.RPCClientConfig{
		ResponsesExchange: settings.RabbitMQ.ConnectorResponsesExchange,
	})
	if err := rpc.Start(ctx); err != nil {
		return fmt.Errorf("connector rpc start: %w", err)
	}
	defer func() { _ = rpc.Close() }()
	log.Info("connector rpc client started",
		slog.String("client_id", rpc.ClientID()),
		slog.String("responses_exchange", settings.RabbitMQ.ConnectorResponsesExchange),
	)

	lakeReader := lakeReaderAdapter{factory: stf}
	connectorRPC := connectors.NewRPCClient(rpc, lakeReader, settings.RabbitMQ.ConnectorCommandsExchange)
	_ = connectorRPC // engines pick this up via DI when they materialise

	// Repositories and services for the integrations layer.
	appsRepo := applications.NewBunRepository(db)
	appsSvc := applications.NewService(appsRepo, eventsSink, nil)

	connRepo := connectors.NewBunRepository(db)
	connSvc := connectors.NewService(connRepo, nil)

	// Registration consumer runs in its own goroutine until ctx fires.
	regChan, err := mq.Conn.Channel()
	if err != nil {
		return fmt.Errorf("registration consumer channel: %w", err)
	}
	defer func() { _ = regChan.Close() }()
	go func() {
		err := connectors.RunRegistrationConsumer(ctx, log, connSvc, connectors.RegistrationConsumerConfig{
			Channel:     regChan,
			Exchange:    settings.RabbitMQ.ConnectorRegistrationExchange,
			Queue:       settings.RabbitMQ.ConnectorRegistrationQueue,
			BindingKeys: []string{"connector.*"},
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("registration consumer terminated", slog.Any("err", err))
		}
	}()
	log.Info("registration consumer running",
		slog.String("exchange", settings.RabbitMQ.ConnectorRegistrationExchange),
		slog.String("queue", settings.RabbitMQ.ConnectorRegistrationQueue),
	)

	e := webserver.New(webserver.Config{
		Debug:            settings.App.Debug,
		CORSAllowOrigins: settings.App.CORSAllowOrigins,
	}, log)

	apiV0 := e.Group("/api/v0")
	applications.RegisterRoutes(apiV0, appsSvc, matchingAdapter{svc: connSvc})
	connectors.RegisterRoutes(apiV0, connSvc)

	// Serve in a goroutine so we can react to signals.
	serveErr := make(chan error, 1)
	go func() {
		log.Info("http listening", slog.String("addr", httpAddr))
		if err := e.Start(httpAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown error", slog.Any("err", err))
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// matchingAdapter bridges applications.MatchingProvider to
// connectors.Service so the applications package never imports
// connectors directly.
type matchingAdapter struct {
	svc *connectors.Service
}

func (m matchingAdapter) MatchingForTags(ctx context.Context, requiredTags []string, onlineOnly bool) (any, error) {
	insts, err := m.svc.MatchingForTags(ctx, requiredTags, onlineOnly)
	if err != nil {
		return nil, err
	}
	out := make([]connectors.InstanceWire, 0, len(insts))
	for _, inst := range insts {
		out = append(out, connectors.NewInstanceWire(inst))
	}
	return out, nil
}

// lakeReaderAdapter implements connectors.LakeReader on top of the
// process-wide storage factory.
type lakeReaderAdapter struct {
	factory *storage.Factory
}

func (a lakeReaderAdapter) ReadBatch(ctx context.Context, provider string, storageKey string) ([]map[string]any, error) {
	s, err := a.factory.Get(provider)
	if err != nil {
		return nil, err
	}
	return s.ReadBatch(ctx, storageKey)
}

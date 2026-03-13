// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command log-siem-transmitter reads the logs topic exchange and emits
// every Event into the configured siem.Sink (file, mq, elk, …).
//
// Stand-alone process — scales independently of backplane and executor.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/joho/godotenv"
)

const (
	logLevel  = "info"
	queueName = "aurelion.logs.siem"
)

// siemProviders are fanned out via siem.MultiSink — every Event goes
// to every listed sink, in order. stdout is excluded on purpose: it
// would write to this consumer's terminal, not the publisher's, which
// is rarely what you want for a transport bridge.
var siemProviders = []string{"file"}

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("startup failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("log-siem-transmitter — MQ → SIEM bridge for log events")
	fmt.Println()
	fmt.Printf("  Reads %s from the aurelion.logs topic exchange.\n", queueName)
	fmt.Println("  Forwards every Event into the configured SIEM sink.")
	fmt.Println("  No HTTP, no buffer — pure delivery path.")
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
	siem.EmitInfo(ctx, bootSink, "log-siem-transmitter", "log-siem-transmitter started")
	defer siem.EmitInfo(context.Background(), bootSink, "log-siem-transmitter", "log-siem-transmitter stopping")

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
	sink := siem.NewMulti(sinks...)
	log.Info("siem sink ready", slog.Any("providers", siemProviders))

	handler := func(ctx context.Context, body []byte) error {
		var ev siem.Event
		if err := json.Unmarshal(body, &ev); err != nil {
			log.Warn("malformed log payload", slog.String("err", err.Error()))
			return nil // drop, don't requeue
		}
		return sink.Emit(ctx, ev)
	}

	log.Info("consuming",
		slog.String("queue", queueName),
		slog.String("exchange", settings.RabbitMQ.LogsExchange),
	)
	return rabbitmq.Consume(ctx, mq.Channel, rabbitmq.ConsumeConfig{
		Exchange:    settings.RabbitMQ.LogsExchange,
		Queue:       queueName,
		BindingKeys: []string{"#"},
		Handler:     handler,
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

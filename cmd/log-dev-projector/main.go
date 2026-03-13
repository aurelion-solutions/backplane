// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command log-dev-projector reads the logs topic exchange into an
// in-memory ring buffer and exposes the recent N over HTTP for local
// development. Not for production — buffer is process-local and lost
// on restart.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/joho/godotenv"
)

const (
	logLevel    = "info"
	queueName   = "aurelion.logs.buffer"
	httpAddr    = ":8001"
	bufferSize  = 1000
	defaultRead = 100
)

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("startup failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("log-dev-projector — in-memory log viewer for local development")
	fmt.Println()
	fmt.Printf("  Reads %s from the aurelion.logs topic exchange.\n", queueName)
	fmt.Println("  Buffers events in memory with FIFO eviction. Restart wipes the buffer.")
	fmt.Println()
	fmt.Printf("  HTTP listening on %s\n", httpAddr)
	fmt.Printf("  curl localhost%s/healthz\n", httpAddr)
	fmt.Printf("  curl 'localhost%s/buffer?limit=50'\n", httpAddr)
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
	siem.EmitInfo(ctx, bootSink, "log-dev-projector", "log-dev-projector started")
	defer siem.EmitInfo(context.Background(), bootSink, "log-dev-projector", "log-dev-projector stopping")

	buf := newRingBuffer(bufferSize)

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           buf.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		log.Info("http listening", slog.String("addr", httpAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	handler := func(_ context.Context, body []byte) error {
		var ev siem.Event
		if err := json.Unmarshal(body, &ev); err != nil {
			log.Warn("malformed log payload", slog.String("err", err.Error()))
			return nil
		}
		buf.push(ev)
		return nil
	}

	log.Info("consuming",
		slog.String("queue", queueName),
		slog.String("exchange", settings.RabbitMQ.LogsExchange),
	)
	consumeErr := make(chan error, 1)
	go func() {
		consumeErr <- rabbitmq.Consume(ctx, mq.Channel, rabbitmq.ConsumeConfig{
			Exchange:    settings.RabbitMQ.LogsExchange,
			Queue:       queueName,
			BindingKeys: []string{"#"},
			Handler:     handler,
		})
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return err
		}
	case err := <-consumeErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(shutdownCtx)
	return nil
}

// ringBuffer is a fixed-capacity, FIFO-evicting Event store.
type ringBuffer struct {
	mu    sync.RWMutex
	data  []siem.Event
	max   int
	count int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{data: make([]siem.Event, 0, max), max: max}
}

func (r *ringBuffer) push(ev siem.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) < r.max {
		r.data = append(r.data, ev)
	} else {
		// drop oldest
		copy(r.data, r.data[1:])
		r.data[len(r.data)-1] = ev
	}
	r.count++
}

func (r *ringBuffer) snapshot(limit int) []siem.Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 || limit > len(r.data) {
		limit = len(r.data)
	}
	out := make([]siem.Event, limit)
	copy(out, r.data[len(r.data)-limit:])
	return out
}

func (r *ringBuffer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /buffer", func(w http.ResponseWriter, req *http.Request) {
		limit := defaultRead
		if raw := req.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		events := r.snapshot(limit)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":  len(events),
			"events": events,
		})
	})
	return mux
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

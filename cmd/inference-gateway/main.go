// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command inference-gateway is the single network entry point for LLM
// inference. Every caller — backplane, worker, pdp — reaches a model
// through this process, never through an in-process provider of its
// own.
//
//	backplane / worker / pdp
//	  -> inference-gateway        (this process)
//	    -> internal/platform/llm provider
//	      -> llama-server or cloud provider
//
// Today the gateway holds one local executor: it builds the provider
// from internal/platform/llm and streams in-process. There is no GPU
// worker pool yet — that lower layer slots in behind the same HTTP
// contract later (see README), so callers never change.
//
// HTTP surface:
//   - POST /v1/inference/stream — Server-Sent Events token stream.
//   - GET  /healthz             — instance metadata + active backend.
//
// Boots like every other binary: AURELION_SECRET_PROVIDER /
// AURELION_SECRETS_FILE → config via the secret manager → wire the llm
// factory → serve. It needs no Postgres, RabbitMQ, or cartridges — the
// gateway is pure transport over the llm platform.
//
// Optional env (defaults shown):
//   - AURELION_INFERENCE_GATEWAY_HTTP_ADDR (:8090) — HTTP bind address.
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
	"syscall"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/webserver"
	"github.com/aurelion-solutions/backplane/internal/platform/llm"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
)

const (
	logLevel        = "info"
	defaultHTTPAddr = ":8090"
)

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("inference-gateway failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("inference-gateway — the single network entry point for LLM inference")
	fmt.Println()
	fmt.Println("  callers (backplane / worker / pdp) stream through here;")
	fmt.Println("  a GPU worker pool slots in behind the same contract later.")
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
		return fmt.Errorf("secret provider: %w", err)
	}
	settings, err := config.Load(sm)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	llf := llm.NewFactory()
	llm.RegisterOpenAI(llf)
	llm.RegisterAnthropic(llf)
	llm.RegisterGemini(llf)
	executor := NewLocalExecutor(llf, settings.LLM)

	active, err := settings.LLM.Active()
	if err != nil {
		return fmt.Errorf("llm active provider: %w", err)
	}
	log.Info("inference-gateway backend",
		slog.String("provider", settings.LLM.Provider),
		slog.String("protocol", active.Protocol),
		slog.String("model", active.Model),
		slog.String("executor", "local"),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	addr := envOr("AURELION_INFERENCE_GATEWAY_HTTP_ADDR", defaultHTTPAddr)
	e := webserver.New(webserver.Config{
		Debug:            settings.App.Debug,
		CORSAllowOrigins: settings.App.CORSAllowOrigins,
	}, log)
	registerHealthRoute(e, settings.LLM)
	registerInferenceRoutes(e, streamDeps{exec: executor, log: log})

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
	log.Info("inference-gateway stopped")
	return nil
}

// registerHealthRoute mounts GET /healthz.
func registerHealthRoute(e *echo.Echo, cfg config.LLM) {
	e.GET("/healthz", func(c echo.Context) error {
		body := map[string]any{
			"status":   "ok",
			"role":     "inference-gateway",
			"executor": "local",
			"provider": cfg.Provider,
		}
		if active, err := cfg.Active(); err == nil {
			body["protocol"] = active.Protocol
			body["model"] = active.Model
		}
		return c.JSON(http.StatusOK, body)
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

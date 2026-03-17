// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command migrate runs the bun migration registry against the
// configured Postgres instance.
//
//	migrate init    # create bun_migrations table (idempotent)
//	migrate up      # apply every unapplied migration
//	migrate down    # revert the most recent applied migration
//	migrate status  # print applied / pending sets
//
// Reads the same secret store as backplane (AURELION_SECRET_PROVIDER /
// AURELION_SECRETS_FILE env vars); every other parameter comes from
// the configured secret manager.
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
	"github.com/aurelion-solutions/backplane/internal/core/postgres"
	"github.com/aurelion-solutions/backplane/internal/migrations"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/joho/godotenv"
	"github.com/uptrace/bun/migrate"
)

const logLevel = "info"

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("migrate failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("migrate — bun migration runner")
	fmt.Println()
	fmt.Println("  Commands: init | up | down | status")
	fmt.Println("  Secret store via AURELION_SECRET_PROVIDER / AURELION_SECRETS_FILE.")
	fmt.Println()
}

func run(log *slog.Logger) error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: migrate <init|up|down|status>")
	}
	cmd := os.Args[1]

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
		return err
	}
	defer func() { _ = db.Close() }()

	migrator := migrate.NewMigrator(db, migrations.Migrations)

	switch cmd {
	case "init":
		if err := migrator.Init(ctx); err != nil {
			return err
		}
		log.Info("migrations table ready")
		return nil
	case "up":
		if err := migrator.Init(ctx); err != nil {
			return err
		}
		group, err := migrator.Migrate(ctx)
		if err != nil {
			return err
		}
		if group.IsZero() {
			log.Info("no new migrations to apply")
			return nil
		}
		log.Info("migrated up", slog.String("group", group.String()))
		return nil
	case "down":
		group, err := migrator.Rollback(ctx)
		if err != nil {
			return err
		}
		if group.IsZero() {
			log.Info("nothing to roll back")
			return nil
		}
		log.Info("rolled back", slog.String("group", group.String()))
		return nil
	case "status":
		ms, err := migrator.MigrationsWithStatus(ctx)
		if err != nil {
			return err
		}
		for _, m := range ms {
			state := "pending"
			if !m.MigratedAt.IsZero() {
				state = "applied " + m.MigratedAt.Format("2006-01-02 15:04:05")
			}
			fmt.Printf("  %-8s %s\n", state, m.Name)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (use init|up|down|status)", cmd)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

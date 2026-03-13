// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package postgres builds a *bun.DB bound to a pgdriver connection
// pool. Mirrors kernel's get_engine + get_session_factory: callers
// receive one DB instance per process and share it.
//
// This package depends on nothing inside backplane (no config, no
// domain). The caller composes a Config and passes it in.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
)

// Config holds the inputs New needs. Callers compose it from whatever
// source they like (bootstrap config, env, tests).
type Config struct {
	DSN   string
	Debug bool
}

// New constructs a verified *bun.DB. Pings the database with the
// supplied context — fail fast at startup if Postgres is unreachable.
func New(ctx context.Context, cfg Config) (*bun.DB, error) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(cfg.DSN)))
	sqldb.SetMaxOpenConns(20)
	sqldb.SetMaxIdleConns(5)
	sqldb.SetConnMaxLifetime(30 * time.Minute)

	db := bun.NewDB(sqldb, pgdialect.New())
	if cfg.Debug {
		db.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return db, nil
}

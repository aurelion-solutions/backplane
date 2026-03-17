// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package migrations is the central registry of bun migrations.
//
// Each migration is a Go file in this package whose init() function
// calls Migrations.MustRegister(up, down). The file name starts with a
// monotonically increasing timestamp prefix (e.g. 20260328000001_*.go);
// bun uses the prefix to determine ordering and stores the applied set
// in the bun_migrations table.
//
// Migrations must NOT import production models — schema changes are
// recorded with raw SQL, so historical migrations keep working after
// the live model evolves.
package migrations

import "github.com/uptrace/bun/migrate"

// Migrations is shared across every migration file in this package.
// cmd/migrate constructs a migrate.Migrator from this registry.
var Migrations = migrate.NewMigrations()

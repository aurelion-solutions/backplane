// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"fmt"
	"net/url"

	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
)

// Postgres holds connection parameters for the primary database.
type Postgres struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// DefaultPostgres returns the local-dev defaults from .env.example.
func DefaultPostgres() Postgres {
	return Postgres{
		Host:     "localhost",
		Port:     5432,
		Database: "aurelion",
		Username: "aurelion",
		Password: "aurelion",
	}
}

// DSN renders a libpq-compatible URL. pgdriver and pgx both accept this.
func (p Postgres) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.Username, p.Password),
		Host:   fmt.Sprintf("%s:%d", p.Host, p.Port),
		Path:   "/" + p.Database,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

func loadPostgres(sm secretmanagers.Manager) (Postgres, error) {
	p := DefaultPostgres()
	if err := decodeRequired(sm, "postgres", &p); err != nil {
		return Postgres{}, err
	}
	return p, nil
}

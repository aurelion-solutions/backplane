// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aurelion-solutions/backplane/internal/core/secret"
)

// Load constructs a Settings snapshot by reading JSON secrets from sm.
// Each section owns its own load<Name> function in its own file; this
// file only orchestrates them.
//
// A missing required key is a fatal startup error. A missing optional
// key silently falls back to the section's defaults.
func Load(sm secret.Manager) (*Settings, error) {
	pg, err := loadPostgres(sm)
	if err != nil {
		return nil, err
	}
	rmq, err := loadRabbitMQ(sm)
	if err != nil {
		return nil, err
	}
	app, err := loadApp(sm)
	if err != nil {
		return nil, err
	}
	return &Settings{
		Postgres: pg,
		RabbitMQ: rmq,
		App:      app,
	}, nil
}

// decodeRequired reads sm[key] and JSON-decodes it into out. A missing
// key is a fatal error.
func decodeRequired(sm secret.Manager, key string, out any) error {
	raw, err := sm.Get(key)
	if err != nil {
		return fmt.Errorf("config: required secret %q: %w", key, err)
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("config: decode secret %q: %w", key, err)
	}
	return nil
}

// decodeOptional reads sm[key] and JSON-decodes it into out. A missing
// key is silently ignored — out keeps the defaults the caller passed in.
func decodeOptional(sm secret.Manager, key string, out any) error {
	raw, err := sm.Get(key)
	if err != nil {
		if errors.Is(err, secret.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("config: optional secret %q: %w", key, err)
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("config: decode secret %q: %w", key, err)
	}
	return nil
}

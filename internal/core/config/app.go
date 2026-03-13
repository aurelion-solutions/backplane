// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package config

import "github.com/aurelion-solutions/backplane/internal/core/secret"

// App is for bootstrap-only app concerns that must exist before any
// HTTP handler runs (CORS, debug flag). Anything runtime-tunable
// belongs in the database, not here.
type App struct {
	Debug            bool     `json:"debug"`
	CORSAllowOrigins []string `json:"cors_allow_origins"`
}

// DefaultApp matches kernel's AppSettings defaults.
func DefaultApp() App {
	return App{
		Debug:            false,
		CORSAllowOrigins: []string{"*"},
	}
}

func loadApp(sm secret.Manager) (App, error) {
	a := DefaultApp()
	if err := decodeOptional(sm, "app", &a); err != nil {
		return App{}, err
	}
	return a, nil
}

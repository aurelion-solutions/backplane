// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"os"

	"github.com/aurelion-solutions/backplane/internal/core/secret"
)

// Cartridges is the bootstrap section for the cartridges-on-disk root.
//
// Resolution priority:
//  1. AURELION_CARTRIDGES_ROOT env var
//  2. secret "cartridges" → JSON {"provider": "...", "root": "..."}
//  3. DefaultCartridges() — provider "filesystem", root "../cartridges"
//     (cartridges live one directory above backplane root by convention)
type Cartridges struct {
	Provider string `json:"provider"`
	Root     string `json:"root"`
}

// DefaultCartridges matches the convention of cartridges/ living one
// directory above backplane root.
func DefaultCartridges() Cartridges {
	return Cartridges{
		Provider: "filesystem",
		Root:     "../cartridges",
	}
}

func loadCartridges(sm secret.Manager) (Cartridges, error) {
	c := DefaultCartridges()
	if err := decodeOptional(sm, "cartridges", &c); err != nil {
		return Cartridges{}, err
	}
	if v := os.Getenv("AURELION_CARTRIDGES_ROOT"); v != "" {
		c.Root = v
	}
	return c, nil
}

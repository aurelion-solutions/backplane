// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secretmanagers

import "github.com/aurelion-solutions/backplane/internal/core/secret"

// Vault is a placeholder for a HashiCorp Vault provider.
type Vault struct{ Stub }

// RegisterVault wires the "vault" stub provider.
func RegisterVault(f *Factory) {
	f.Register("vault", func() (secret.FullManager, error) { return Vault{}, nil })
}

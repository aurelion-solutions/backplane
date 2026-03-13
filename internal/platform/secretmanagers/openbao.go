// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secretmanagers

import "github.com/aurelion-solutions/backplane/internal/core/secret"

// OpenBao is a placeholder for an OpenBao (Vault fork) provider.
type OpenBao struct{ Stub }

// RegisterOpenBao wires the "openbao" stub provider.
func RegisterOpenBao(f *Factory) {
	f.Register("openbao", func() (secret.FullManager, error) { return OpenBao{}, nil })
}

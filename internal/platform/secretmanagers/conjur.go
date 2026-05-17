// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secretmanagers

// Conjur is a placeholder for a CyberArk Conjur provider.
type Conjur struct{ Stub }

// RegisterConjur wires the "conjur" stub provider.
func RegisterConjur(f *Factory) {
	f.Register("conjur", func() (FullManager, error) { return Conjur{}, nil })
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secretmanagers

// Akeyless is a placeholder for an Akeyless provider.
type Akeyless struct{ Stub }

// RegisterAkeyless wires the "akeyless" stub provider.
func RegisterAkeyless(f *Factory) {
	f.Register("akeyless", func() (FullManager, error) { return Akeyless{}, nil })
}

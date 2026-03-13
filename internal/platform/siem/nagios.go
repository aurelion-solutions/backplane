// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// Nagios is a placeholder for a Nagios passive-check sink.
type Nagios struct{ Stub }

// RegisterNagios wires the "nagios" stub provider.
func RegisterNagios(f *Factory) {
	f.Register("nagios", func() (Sink, error) { return Nagios{}, nil })
}

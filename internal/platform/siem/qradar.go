// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// QRadar is a placeholder for an IBM QRadar LEEF / Syslog sink.
type QRadar struct{ Stub }

// RegisterQRadar wires the "qradar" stub provider.
func RegisterQRadar(f *Factory) {
	f.Register("qradar", func() (Sink, error) { return QRadar{}, nil })
}

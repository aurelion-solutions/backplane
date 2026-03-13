// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// Loki is a placeholder for a Grafana Loki push-API sink.
type Loki struct{ Stub }

// RegisterLoki wires the "loki" stub provider.
func RegisterLoki(f *Factory) {
	f.Register("loki", func() (Sink, error) { return Loki{}, nil })
}

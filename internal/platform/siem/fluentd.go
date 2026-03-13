// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// Fluentd is a placeholder for a Fluentd forward-protocol sink.
type Fluentd struct{ Stub }

// RegisterFluentd wires the "fluentd" stub provider.
func RegisterFluentd(f *Factory) {
	f.Register("fluentd", func() (Sink, error) { return Fluentd{}, nil })
}

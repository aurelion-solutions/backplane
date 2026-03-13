// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// ELK is a placeholder for an Elasticsearch / Logstash / Kibana sink.
type ELK struct{ Stub }

// RegisterELK wires the "elk" stub provider.
func RegisterELK(f *Factory) {
	f.Register("elk", func() (Sink, error) { return ELK{}, nil })
}

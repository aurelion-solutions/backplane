// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// Splunk is a placeholder for a Splunk HEC (HTTP Event Collector) sink.
type Splunk struct{ Stub }

// RegisterSplunk wires the "splunk" stub provider.
func RegisterSplunk(f *Factory) {
	f.Register("splunk", func() (Sink, error) { return Splunk{}, nil })
}

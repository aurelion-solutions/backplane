// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// Seq is a placeholder for a Datalust Seq ingestion-API sink.
type Seq struct{ Stub }

// RegisterSeq wires the "seq" stub provider.
func RegisterSeq(f *Factory) {
	f.Register("seq", func() (Sink, error) { return Seq{}, nil })
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package storage

// Iceberg is a placeholder for an Apache Iceberg table-format backend.
type Iceberg struct{ Stub }

// RegisterIceberg wires the "iceberg" stub provider.
func RegisterIceberg(f *Factory) {
	f.Register("iceberg", func() (Storage, error) { return Iceberg{}, nil })
}

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package storage

// S3 is a placeholder for an S3-compatible object storage backend.
type S3 struct{ Stub }

// RegisterS3 wires the "s3" stub provider.
func RegisterS3(f *Factory) {
	f.Register("s3", func() (Storage, error) { return S3{}, nil })
}

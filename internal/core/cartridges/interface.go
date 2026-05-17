// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import "errors"

// ErrNotFound is returned when the requested cartridge id is unknown
// to the provider.
var ErrNotFound = errors.New("cartridges: not found")

// ErrInvalidManifest is returned when a .meta.json sidecar fails to
// parse or violates the manifest contract.
var ErrInvalidManifest = errors.New("cartridges: invalid manifest")

// Ref is the logical identifier of a cartridge.
//
// ID is the cartridge's stable name — the top-level directory under the
// cartridges root for FilesystemProvider, the git remote slug for a
// future GitProvider, the OCI image name for a future OCIProvider.
//
// Version is opaque to backplane. Filesystem provider leaves it empty;
// providers that have versioning (git ref, OCI tag) populate it.
type Ref struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// Provider is the source-agnostic cartridge access contract.
//
// Implementations enumerate cartridges they can serve and materialize
// each one onto a local directory path. Returning a local path is
// intentional — every downstream consumer (pipeline YAML loader, OPA
// sidecar) wants files on disk, so providers that fetch from elsewhere
// (git, OCI, zip) extract into a cached directory and return its root.
type Provider interface {
	// List returns refs for every cartridge this provider can serve,
	// sorted by ID.
	List() ([]Ref, error)

	// Materialize returns the local directory root that holds the
	// cartridge's files.
	//
	// Returns ErrNotFound if ref.ID is unknown.
	Materialize(ref Ref) (string, error)

	// Policies returns rule_id → manifest for every policy in the
	// cartridge. Subdirectories under policies/ are recursed.
	//
	// Returns ErrNotFound if ref.ID is unknown.
	Policies(ref Ref) (map[string]Manifest, error)

	// Pipelines returns the list of pipeline YAML file paths under
	// the cartridge's pipelines/ directory.
	//
	// The returned paths are absolute on disk. Returns an empty slice
	// when the cartridge has no pipelines/ directory.
	Pipelines(ref Ref) ([]string, error)

	// Apps returns app_id → cartridge for every directory under the
	// bundle's apps/ tree. Each AppCartridge has its three YAML files
	// (manifest, account, descriptor) loaded and validated.
	//
	// Returns ErrNotFound if ref.ID is unknown. Returns an empty map
	// when the bundle has no apps/ directory.
	Apps(ref Ref) (map[string]AppCartridge, error)
}

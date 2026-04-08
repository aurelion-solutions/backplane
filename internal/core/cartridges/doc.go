// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package cartridges is the source-agnostic provider for the cartridge
// bundle that lives outside the backplane git repo (default:
// ../cartridges relative to backplane root, overridable via
// AURELION_CARTRIDGES_ROOT).
//
// A cartridge is a directory of static files:
//
//	<cartridges-root>/<id>/
//	    pipelines/*.yaml          ← orchestrator definitions
//	    policies/<bucket>/*.rego  ← OPA rules (paired with .meta.json)
//	    policies/<bucket>/*.meta.json
//
// This package knows nothing about pipeline grammar or rego semantics —
// it only enumerates cartridge ids and materializes files on disk.
// Higher layers (orchestrator, policy_assessment when it lands) parse
// the contents.
//
// The Provider interface is deliberately generic so a future git/oci/zip
// implementation can drop in alongside FilesystemProvider without
// touching callers.
package cartridges

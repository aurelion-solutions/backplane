// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package loader parses pipeline YAML definitions out of cartridge
// pipelines/ directories and validates them against the embedded JSON
// Schema grammar (see internal/core/orchestrator/grammar).
//
// Validation is fail-fast and runs in a fixed order so error messages
// stay deterministic:
//
//  1. YAML parse → mapping
//  2. JSON Schema (structural grammar)
//  3. requires graph (no forward / self / unknown refs)
//  4. templating (${args.X} declared, ${steps.S.result.X} reachable
//     through the transitive requires closure)
//  5. triggers (at most one schedule, mq args_from_payload mapping
//     valid)
//  6. content_hash + build of the immutable Definition value
//
// Action-reference validation against the live ActionRegistry is
// optional and disabled by default (Step 2 has no registry yet — it
// arrives in Step 3). Discovery wires the registry in on Step 5.
package loader

// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package descriptor renders an app cartridge's `descriptor.yaml`
// recipe into a concrete account descriptor — a `map[string]any` keyed
// by field name with values ready to be handed to a connector.
//
// Inputs:
//
//   - the parsed cartridges.AppCartridge
//   - a Principal (any value — typically the inventory record)
//   - an Application map (from manifest.yaml `config:`)
//   - a target account state (one of the states declared in account.yaml)
//
// Pipeline applied to every field:
//
//  1. Pick the template — `template` directly, or `by_state[target]`.
//     A `by_state` field that has no entry for the target state is
//     omitted from the output entirely.
//  2. Execute the Go `text/template` against the bindings
//     (`.Principal`, `.Application`, `.Descriptor`). Missing keys are
//     strict — referencing an undefined descriptor field is an error,
//     not the literal `<no value>`.
//  3. Apply post-template transforms in declaration order
//     (`remove_diacritics`, `truncate:N`, …).
//  4. If `on_collision` is set, hand the value to the named Resolver
//     from the registry. The default resolver is a no-op stub that
//     returns the value unchanged.
//
// Cross-field references via `.Descriptor.<name>` are resolved by
// topologically sorting fields at NewRenderer time. Cycles and
// references to undeclared fields are rejected up front, not at
// render time.
package descriptor

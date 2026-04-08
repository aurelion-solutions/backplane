// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package registry is the in-memory keyed registry of engine actions.
//
// Engines register handlers at composition time (cmd/backplane,
// cmd/worker) via the generic Register helper. The runner dispatches
// via (engine, action) and the registry takes care of:
//
//	* args JSON Schema validation (santhosh-tekuri/jsonschema)
//	* args JSON → handler input struct unmarshalling
//	* handler invocation under the supplied ActionContext
//	* result JSON Schema validation
//
// JSON Schemas are derived from handler input / output struct
// definitions via github.com/invopop/jsonschema. No hand-written
// schemas are accepted.
//
// Layering: registry imports core deps only. Action handlers shipped
// alongside engines (e.g. internal/actions/noop) live in their own
// packages and call Register at composition time.
package registry

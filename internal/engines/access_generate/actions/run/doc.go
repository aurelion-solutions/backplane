// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package run is the orchestrator-side wrapper around
// access_generate.Engine.Recompute.
//
// Pipeline YAML uses it like this:
//
//	steps:
//	  - name: regenerate
//	    engine: access_generate
//	    action: run
//	    args:
//	      principal_id: "{{ event.principal_id }}"
//	      # optional filters:
//	      application_id: "..."
//	      capability_id:  "..."
//
// Composition root constructs the engine once and hands it to the
// action through Deps. The action itself is thin — parse ids, call
// Recompute, fold the result into JSON counters for observability.
package run

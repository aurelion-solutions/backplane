// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package policy_assessment is the engine that every caller (PDP AuthZ
// transport, AuthN transports, the policy-assessment action running in
// the worker) goes through to evaluate one or many policies.
//
// One engine. One dispatcher. N mechanism handlers — each handler owns
// one class-of-evaluation problem end-to-end (cedar for AuthZ, sod for
// combinatorial detection, llm_classification for prompt-driven
// classification, etc.). Callers do not know which mechanism backs a
// given policy; they hand the engine a request and aggregate outputs.
//
// See README.md for the engine ownership rules and the mechanism index.
package policy_assessment

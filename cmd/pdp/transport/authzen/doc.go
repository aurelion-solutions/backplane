// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package authzen is the PDP transport for the OpenID AuthZen 1.0
// Access Evaluation API.
//
// It is a thin HTTP adapter:
//
//  1. Parse the AuthZen request envelope.
//  2. Derive facets from envelope fields + `context.*`. The caller
//     (IdP / API gateway) is responsible for populating context with
//     transport / geo / device / etc. facets — PDP does not guess.
//  3. Ask the engine Store for policies whose tags are a subset of
//     facets (coarse pre-filter).
//  4. Dispatch each selected policy through the policy_assessment
//     dispatcher.
//  5. Aggregate with deny-wins + obligation union.
//  6. Marshal AuthZen response.
package authzen

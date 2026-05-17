// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package secretmanagers holds the secret-store contracts plus every
// shipped backend (File, Vault, OpenBao, Akeyless, Conjur, …).
//
// Three composable interfaces:
//
//	Manager      — Get only (bootstrap-config reads it).
//	Mutator      — Set + Delete (runtime capability that manages secrets).
//	FullManager  — Manager + Mutator (what providers implement).
//
// Consumers depend on the narrowest contract they need:
//   - config.Load takes a Manager — it cannot mutate.
//   - a future identity-provisioning capability takes a Mutator — it
//     cannot read secrets back.
//
// Providers register against Factory at composition time; the runtime
// resolves one provider by name from settings.
package secretmanagers

import "errors"

// ErrNotFound is returned when the requested key is absent.
var ErrNotFound = errors.New("secret: key not found")

// ErrNotImplemented is returned by stub providers whose backend
// integration has not been written yet.
var ErrNotImplemented = errors.New("secret: provider not implemented")

// Manager is the read-only contract used by bootstrap config.
type Manager interface {
	Get(key string) (string, error)
}

// Mutator is the write-only contract used by runtime capabilities.
type Mutator interface {
	Set(key, value string) error
	Delete(key string) error
}

// FullManager is the union — every provider must satisfy this.
// Callers that need just one half should depend on Manager or Mutator.
type FullManager interface {
	Manager
	Mutator
}

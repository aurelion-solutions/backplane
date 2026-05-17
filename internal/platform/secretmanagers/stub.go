// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secretmanagers

// Stub is a no-op FullManager whose every method returns
// ErrNotImplemented. Embed it in a provider whose real transport has
// not been wired yet.
type Stub struct{}

// Get implements Manager.
func (Stub) Get(_ string) (string, error) { return "", ErrNotImplemented }

// Set implements Mutator.
func (Stub) Set(_, _ string) error { return ErrNotImplemented }

// Delete implements Mutator.
func (Stub) Delete(_ string) error { return ErrNotImplemented }

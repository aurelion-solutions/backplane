// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secretmanagers

import "github.com/aurelion-solutions/backplane/internal/core/secret"

// Stub is a no-op FullManager whose every method returns
// secret.ErrNotImplemented. Embed it in a provider whose real
// transport has not been wired yet.
type Stub struct{}

// Get implements secret.Manager.
func (Stub) Get(_ string) (string, error) { return "", secret.ErrNotImplemented }

// Set implements secret.Mutator.
func (Stub) Set(_, _ string) error { return secret.ErrNotImplemented }

// Delete implements secret.Mutator.
func (Stub) Delete(_ string) error { return secret.ErrNotImplemented }

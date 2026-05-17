// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package descriptor

import "context"

// ResolverInput is the value plus context handed to an on_collision
// resolver after the template+transforms pipeline produced the
// candidate.
type ResolverInput struct {
	Field       string
	Value       string
	Principal   any
	Application map[string]any
}

// Resolver picks the final value when the rendered+transformed
// candidate would collide with something already in storage.
// Implementations may hit the database.
type Resolver interface {
	Resolve(ctx context.Context, in ResolverInput) (string, error)
}

// ResolverFunc adapts a plain function to Resolver.
type ResolverFunc func(ctx context.Context, in ResolverInput) (string, error)

// Resolve calls f.
func (f ResolverFunc) Resolve(ctx context.Context, in ResolverInput) (string, error) {
	return f(ctx, in)
}

// ResolverRegistry maps an on_collision name from the YAML recipe to
// an implementation. The renderer falls back to StubResolver when a
// field references a name not present in the registry.
type ResolverRegistry map[string]Resolver

// StubResolver is the no-op resolver — returns the input value
// unchanged. Used when the actual collision check is not wired up
// yet.
var StubResolver Resolver = ResolverFunc(func(_ context.Context, in ResolverInput) (string, error) {
	return in.Value, nil
})

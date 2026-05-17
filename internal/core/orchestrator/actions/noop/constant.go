// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import (
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
)

// ConstantArgs is the input contract for noop.constant. The Value is
// an arbitrary JSON object that is copied verbatim into the result.
// Use it to stand up downstream template expressions
// (${steps.X.result.value...}) before the real producing action exists.
// Value is optional — omitting it yields an empty object so the action
// can also act as a pure no-op step.
type ConstantArgs struct {
	Value map[string]any `json:"value,omitempty"`
}

// ConstantResult mirrors the input Value byte-for-byte.
type ConstantResult struct {
	Value map[string]any `json:"value"`
}

func constant(args ConstantArgs, _ registry.ActionContext) (ConstantResult, error) {
	if args.Value == nil {
		return ConstantResult{Value: map[string]any{}}, nil
	}
	return ConstantResult{Value: args.Value}, nil
}

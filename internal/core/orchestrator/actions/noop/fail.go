// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import (
	"errors"
	"fmt"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
)

// FailArgs is the input contract for noop.fail. Message is required —
// an empty string is rejected so a misconfigured cartridge surfaces
// loudly instead of producing a blank failure.
type FailArgs struct {
	Message string `json:"message"`
}

// FailResult is empty; the action never returns success.
type FailResult struct{}

// ErrDeliberate is the sentinel error noop.fail raises with the
// supplied message wrapped in. Callers inspecting handler errors can
// match on it with errors.Is.
var ErrDeliberate = errors.New("noop.fail: deliberate failure")

func failAction(args FailArgs, _ registry.ActionContext) (FailResult, error) {
	if args.Message == "" {
		return FailResult{}, fmt.Errorf("noop.fail: message must not be empty")
	}
	return FailResult{}, fmt.Errorf("%w: %s", ErrDeliberate, args.Message)
}

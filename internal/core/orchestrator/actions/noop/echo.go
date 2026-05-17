// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import (
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
)

// EchoArgs is the input contract for noop.echo.
type EchoArgs struct {
	Message string `json:"message"`
}

// EchoResult is the output contract for noop.echo. The message is
// returned verbatim so downstream steps can template
// ${steps.echo.result.message}.
type EchoResult struct {
	Message string `json:"message"`
}

func echo(args EchoArgs, _ registry.ActionContext) (EchoResult, error) {
	return EchoResult{Message: args.Message}, nil
}

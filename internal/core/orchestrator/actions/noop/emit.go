// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import (
	"fmt"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
)

// EmitArgs is the input contract for noop.emit. EventType is required
// and must satisfy the events.NewEnvelope grammar (dotted lowercase,
// see internal/core/events). CorrelationID is optional — when empty
// the action falls back to the pipeline run ID, so emitted envelopes
// always carry a non-empty correlation chain. Payload is copied
// verbatim.
type EmitArgs struct {
	EventType     string         `json:"event_type"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
}

// EmitResult reports the envelope identifiers that landed on the
// exchange. Useful for downstream `wait_for_event` matching by
// event_id.
type EmitResult struct {
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	CorrelationID string `json:"correlation_id"`
}

func emit(args EmitArgs, ctx registry.ActionContext) (EmitResult, error) {
	if ctx.Events == nil {
		return EmitResult{}, fmt.Errorf("noop.emit: events sink not wired into ActionContext")
	}
	corr := args.CorrelationID
	if corr == "" {
		corr = ctx.PipelineRunID.String()
	}
	env, err := events.NewEnvelope(events.EnvelopeInput{
		EventType:     args.EventType,
		CorrelationID: corr,
		Payload:       args.Payload,
	})
	if err != nil {
		return EmitResult{}, fmt.Errorf("noop.emit: %w", err)
	}
	if err := ctx.Events.Emit(ctx.Ctx, env); err != nil {
		return EmitResult{}, fmt.Errorf("noop.emit: sink: %w", err)
	}
	return EmitResult{
		EventID:       env.EventID.String(),
		EventType:     env.EventType,
		CorrelationID: env.CorrelationID,
	}, nil
}

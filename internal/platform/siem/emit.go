// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

import "context"

// EmitInfo sends an informational Event through sink with the given
// component and message. Used for process lifecycle heartbeats
// (started / stopping) — failures are silently dropped so log
// transport hiccups never abort the host process.
func EmitInfo(ctx context.Context, sink Sink, component, message string) {
	ev, err := NewRoot(RootInput{
		Level:         LevelInfo,
		Message:       message,
		Component:     component,
		InitiatorType: ParticipantSystem,
		InitiatorID:   "process",
		ActorType:     ParticipantSystem,
		ActorID:       component,
		TargetType:    ParticipantSystem,
		TargetID:      component,
	})
	if err != nil {
		return
	}
	_ = sink.Emit(ctx, ev)
}

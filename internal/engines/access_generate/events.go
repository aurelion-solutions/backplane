// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"time"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/inventory/initiatives"
)

// EventSink is the MQ contract Engine needs. Aliased to the platform
// `events.Sink` so the composition root passes its real sink in
// directly — no adapter needed.
type EventSink = events.Sink

// Topic names for the two MQ events this engine emits. Kept as
// constants so downstream consumers (PDP validator, future
// access_promote) can subscribe by symbol rather than string
// literal.
const (
	TopicInitiativeCreated    = "inventory.initiative.created"
	TopicInitiativeTombstoned = "inventory.initiative.tombstoned"
)

// PendingEvent is what Recompute stages during the transaction and
// publishes after commit. Keeping the publish step out of the tx
// avoids the well-known "DB committed but broker down" failure mode
// where MQ subscribers see a row that does not exist yet.
type PendingEvent struct {
	Topic    string
	Envelope events.EnvelopeInput
}

func eventCreated(i *initiatives.Initiative, correlationID string) PendingEvent {
	payload := map[string]any{
		"initiative_id":  i.ID.String(),
		"principal_id":   i.PrincipalID.String(),
		"application_id": i.ApplicationID.String(),
		"kind":           i.Kind,
		"actor":          i.Actor,
	}
	if i.CapabilityID != nil {
		payload["capability_id"] = i.CapabilityID.String()
	}
	return PendingEvent{
		Topic: TopicInitiativeCreated,
		Envelope: events.EnvelopeInput{
			EventType:     TopicInitiativeCreated,
			CorrelationID: correlationID,
			OccurredAt:    time.Now().UTC(),
			Payload:       payload,
			TargetID:      i.ID.String(),
		},
	}
}

func eventTombstoned(i *initiatives.Initiative, correlationID string) PendingEvent {
	payload := map[string]any{
		"initiative_id":  i.ID.String(),
		"principal_id":   i.PrincipalID.String(),
		"application_id": i.ApplicationID.String(),
		"kind":           i.Kind,
	}
	if i.CapabilityID != nil {
		payload["capability_id"] = i.CapabilityID.String()
	}
	return PendingEvent{
		Topic: TopicInitiativeTombstoned,
		Envelope: events.EnvelopeInput{
			EventType:     TopicInitiativeTombstoned,
			CorrelationID: correlationID,
			OccurredAt:    time.Now().UTC(),
			Payload:       payload,
			TargetID:      i.ID.String(),
		},
	}
}

// newCorrelationID returns a fresh correlation id stamped onto every
// envelope emitted from one Recompute pass, so consumers can tie the
// creates and tombstones from a single run together.
func newCorrelationID() string { return uuid.NewString() }

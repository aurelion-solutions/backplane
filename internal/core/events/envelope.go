// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package events delivers domain Envelopes to event transports
// (RabbitMQ, noop, tee fan-out). One package holds the contract
// (Sink, Envelope), the Factory registry, and every provider.
//
// This is NOT process logging (see core/logger) and NOT audit logs
// (see platform/siem). An Envelope is what the system DECIDED — a
// fact other services and capabilities react to.
package events

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// eventTypeRe enforces the canonical "<domain>.<entity>.<operation>"
// shape, all-lowercase ASCII with underscores allowed inside segments.
var eventTypeRe = regexp.MustCompile(`^[a-z0-9_]+\.[a-z0-9_]+\.[a-z0-9_]+$`)

// ParticipantKind classifies the initiator, actor, and target of an Envelope.
// All three are optional on an Envelope — producers fill in what they know.
type ParticipantKind string

const (
	ParticipantSystem      ParticipantKind = "system"
	ParticipantUser        ParticipantKind = "user"
	ParticipantConnector   ParticipantKind = "connector"
	ParticipantCapability  ParticipantKind = "capability"
	ParticipantComponent   ParticipantKind = "component"
	ParticipantApplication ParticipantKind = "application"
)

// Valid reports whether the kind matches a defined constant.
// The zero value ("") is treated as "not set" and is also Valid here —
// callers decide whether absence is allowed via Envelope construction.
func (k ParticipantKind) Valid() bool {
	switch k {
	case "", ParticipantSystem, ParticipantUser, ParticipantConnector,
		ParticipantCapability, ParticipantComponent, ParticipantApplication:
		return true
	}
	return false
}

// Envelope is an immutable domain-event record published to the
// platform events exchange. Routing key == EventType byte-for-byte.
//
// Construct via NewEnvelope rather than building the struct literal
// directly — only the constructor enforces the event_type grammar.
type Envelope struct {
	EventID       uuid.UUID       `json:"event_id"`
	EventType     string          `json:"event_type"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID string          `json:"correlation_id"`
	CausationID   *uuid.UUID      `json:"causation_id,omitempty"`
	Payload       map[string]any  `json:"payload,omitempty"`
	InitiatorKind ParticipantKind `json:"initiator_kind,omitempty"`
	InitiatorID   string          `json:"initiator_id,omitempty"`
	ActorKind     ParticipantKind `json:"actor_kind,omitempty"`
	ActorID       string          `json:"actor_id,omitempty"`
	TargetKind    ParticipantKind `json:"target_kind,omitempty"`
	TargetID      string          `json:"target_id,omitempty"`
	SchemaVersion string          `json:"schema_version"`
}

// EnvelopeInput is the parameter struct for NewEnvelope. Required
// fields are EventType and CorrelationID. Everything else has a
// zero-value sentinel ("auto-generate" / "not set"). Pass CausationID
// as uuid.Nil to omit it; pass a real UUID to set it.
type EnvelopeInput struct {
	EventType     string
	CorrelationID string

	OccurredAt    time.Time
	EventID       uuid.UUID
	CausationID   uuid.UUID
	Payload       map[string]any
	InitiatorKind ParticipantKind
	InitiatorID   string
	ActorKind     ParticipantKind
	ActorID       string
	TargetKind    ParticipantKind
	TargetID      string
	SchemaVersion string
}

// NewEnvelope builds and validates an Envelope. Generates EventID,
// OccurredAt, and SchemaVersion when their input zero values signal
// "auto-fill". Rejects invalid event_type, empty CorrelationID,
// self-referential causation, and unknown ParticipantKind values.
func NewEnvelope(in EnvelopeInput) (Envelope, error) {
	if !eventTypeRe.MatchString(in.EventType) {
		return Envelope{}, fmt.Errorf("events: event_type must match <domain>.<entity>.<operation>, got %q", in.EventType)
	}
	cid := strings.TrimSpace(in.CorrelationID)
	if cid == "" {
		return Envelope{}, errors.New("events: correlation_id must be a non-empty string")
	}
	for label, k := range map[string]ParticipantKind{
		"initiator_kind": in.InitiatorKind,
		"actor_kind":     in.ActorKind,
		"target_kind":    in.TargetKind,
	} {
		if !k.Valid() {
			return Envelope{}, fmt.Errorf("events: invalid %s %q", label, k)
		}
	}
	for label, id := range map[string]string{
		"initiator_id": in.InitiatorID,
		"actor_id":     in.ActorID,
		"target_id":    in.TargetID,
	} {
		if id != "" && strings.TrimSpace(id) == "" {
			return Envelope{}, fmt.Errorf("events: %s must be non-empty if set", label)
		}
	}

	eid := in.EventID
	if eid == uuid.Nil {
		eid = uuid.New()
	}
	var causation *uuid.UUID
	if in.CausationID != uuid.Nil {
		cp := in.CausationID
		if cp == eid {
			return Envelope{}, errors.New("events: causation_id must not equal event_id")
		}
		causation = &cp
	}
	occurred := in.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	} else {
		occurred = occurred.UTC()
	}
	schema := in.SchemaVersion
	if schema == "" {
		schema = "1"
	}

	return Envelope{
		EventID:       eid,
		EventType:     in.EventType,
		OccurredAt:    occurred,
		CorrelationID: cid,
		CausationID:   causation,
		Payload:       cloneMap(in.Payload),
		InitiatorKind: in.InitiatorKind,
		InitiatorID:   in.InitiatorID,
		ActorKind:     in.ActorKind,
		ActorID:       in.ActorID,
		TargetKind:    in.TargetKind,
		TargetID:      in.TargetID,
		SchemaVersion: schema,
	}, nil
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

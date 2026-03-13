// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package logsink delivers structured business / audit events to
// external observability backends (file, MQ, SIEM). One package holds
// the contract (Sink, Reader, Event), the Factory registry, and every
// provider implementation.
//
// This is NOT the process-side logger (see core/logger). Sink is
// the destination for events that describe what the system DID:
// "who acted on whom, when, with which correlation chain".
//
// Adding a real backend: replace the Stub-based ELK / Loki / Splunk /
// etc. with a real type that implements Sink. No other file changes.
package siem

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Level is the allowed log level for an Event.
type Level string

const (
	LevelDebug    Level = "debug"
	LevelInfo     Level = "info"
	LevelWarning  Level = "warning"
	LevelError    Level = "error"
	LevelCritical Level = "critical"
)

// Valid reports whether the level matches a defined constant.
func (l Level) Valid() bool {
	switch l {
	case LevelDebug, LevelInfo, LevelWarning, LevelError, LevelCritical:
		return true
	}
	return false
}

// ParticipantKind classifies the initiator, actor, and target of an Event.
type ParticipantKind string

const (
	ParticipantSystem      ParticipantKind = "system"
	ParticipantUser        ParticipantKind = "user"
	ParticipantConnector   ParticipantKind = "connector"
	ParticipantCapability  ParticipantKind = "capability"
	ParticipantApplication ParticipantKind = "application"
)

// Valid reports whether the kind matches a defined constant.
func (k ParticipantKind) Valid() bool {
	switch k {
	case ParticipantSystem, ParticipantUser, ParticipantConnector,
		ParticipantCapability, ParticipantApplication:
		return true
	}
	return false
}

// Event is a structured log record routed through Sink. Frozen by
// convention — construct via NewRoot / NewDownstream / NewDownstreamFromParentID
// rather than building the struct literal directly.
//
// Semantics:
//   - initiator: who wanted or started the action.
//   - actor:     who executes the current step.
//   - target:    what the action is performed on.
//
// Propagation:
//   - A root event generates a new EventID and CorrelationID; CausationID is nil.
//   - A downstream event generates a new EventID, preserves the parent's
//     CorrelationID, and sets CausationID to the parent's EventID.
type Event struct {
	EventID       uuid.UUID       `json:"event_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Level         Level           `json:"level"`
	Message       string          `json:"message"`
	Component     string          `json:"component"`
	CorrelationID string          `json:"correlation_id"`
	CausationID   *uuid.UUID      `json:"causation_id,omitempty"`
	Payload       map[string]any  `json:"payload,omitempty"`
	InitiatorType ParticipantKind `json:"initiator_type"`
	InitiatorID   string          `json:"initiator_id"`
	ActorType     ParticipantKind `json:"actor_type"`
	ActorID       string          `json:"actor_id"`
	TargetType    ParticipantKind `json:"target_type"`
	TargetID      string          `json:"target_id"`
}

// RootInput is the parameter struct for NewRoot. Optional fields are
// zero-value sentinels: nil EventID means "generate one", empty
// CorrelationID means "generate one", zero Timestamp means "now".
type RootInput struct {
	Level         Level
	Message       string
	Component     string
	InitiatorType ParticipantKind
	InitiatorID   string
	ActorType     ParticipantKind
	ActorID       string
	TargetType    ParticipantKind
	TargetID      string

	Payload       map[string]any
	Timestamp     time.Time
	EventID       uuid.UUID
	CorrelationID string
}

// DownstreamInput is the parameter struct for NewDownstream. Same
// optional semantics as RootInput. CorrelationID is taken from the
// parent and cannot be overridden.
type DownstreamInput struct {
	Level         Level
	Message       string
	Component     string
	InitiatorType ParticipantKind
	InitiatorID   string
	ActorType     ParticipantKind
	ActorID       string
	TargetType    ParticipantKind
	TargetID      string

	Payload   map[string]any
	Timestamp time.Time
	EventID   uuid.UUID
}

// NewRoot builds a trace-root Event. A new EventID and CorrelationID
// are generated when the corresponding fields in RootInput are zero.
func NewRoot(in RootInput) (Event, error) {
	e := Event{
		EventID:       chooseUUID(in.EventID),
		Timestamp:     chooseTime(in.Timestamp),
		Level:         in.Level,
		Message:       in.Message,
		Component:     in.Component,
		CorrelationID: chooseCorrelationID(in.CorrelationID),
		CausationID:   nil,
		Payload:       cloneMap(in.Payload),
		InitiatorType: in.InitiatorType,
		InitiatorID:   in.InitiatorID,
		ActorType:     in.ActorType,
		ActorID:       in.ActorID,
		TargetType:    in.TargetType,
		TargetID:      in.TargetID,
	}
	if err := e.validate(); err != nil {
		return Event{}, err
	}
	return e, nil
}

// NewDownstream builds an Event that inherits CorrelationID from parent
// and sets CausationID to the parent's EventID.
func NewDownstream(parent Event, in DownstreamInput) (Event, error) {
	parentID := parent.EventID
	e := Event{
		EventID:       chooseUUID(in.EventID),
		Timestamp:     chooseTime(in.Timestamp),
		Level:         in.Level,
		Message:       in.Message,
		Component:     in.Component,
		CorrelationID: parent.CorrelationID,
		CausationID:   &parentID,
		Payload:       cloneMap(in.Payload),
		InitiatorType: in.InitiatorType,
		InitiatorID:   in.InitiatorID,
		ActorType:     in.ActorType,
		ActorID:       in.ActorID,
		TargetType:    in.TargetType,
		TargetID:      in.TargetID,
	}
	if err := e.validate(); err != nil {
		return Event{}, err
	}
	return e, nil
}

// NewDownstreamFromParentID builds a downstream Event when only the
// parent's EventID and CorrelationID are known (e.g. connector RPC).
func NewDownstreamFromParentID(parentEventID uuid.UUID, correlationID string, in DownstreamInput) (Event, error) {
	if strings.TrimSpace(correlationID) == "" {
		return Event{}, errors.New("logsink: correlation_id must be a non-empty string")
	}
	pid := parentEventID
	e := Event{
		EventID:       chooseUUID(in.EventID),
		Timestamp:     chooseTime(in.Timestamp),
		Level:         in.Level,
		Message:       in.Message,
		Component:     in.Component,
		CorrelationID: correlationID,
		CausationID:   &pid,
		Payload:       cloneMap(in.Payload),
		InitiatorType: in.InitiatorType,
		InitiatorID:   in.InitiatorID,
		ActorType:     in.ActorType,
		ActorID:       in.ActorID,
		TargetType:    in.TargetType,
		TargetID:      in.TargetID,
	}
	if err := e.validate(); err != nil {
		return Event{}, err
	}
	return e, nil
}

func (e Event) validate() error {
	if !e.Level.Valid() {
		return fmt.Errorf("logsink: invalid level %q", e.Level)
	}
	if strings.TrimSpace(e.Message) == "" {
		return errors.New("logsink: message must be a non-empty string")
	}
	if strings.TrimSpace(e.Component) == "" {
		return errors.New("logsink: component must be a non-empty string")
	}
	if strings.TrimSpace(e.CorrelationID) == "" {
		return errors.New("logsink: correlation_id must be a non-empty string")
	}
	for label, p := range map[string]ParticipantKind{
		"initiator_type": e.InitiatorType,
		"actor_type":     e.ActorType,
		"target_type":    e.TargetType,
	} {
		if !p.Valid() {
			return fmt.Errorf("logsink: invalid %s %q", label, p)
		}
	}
	for label, id := range map[string]string{
		"initiator_id": e.InitiatorID,
		"actor_id":     e.ActorID,
		"target_id":    e.TargetID,
	} {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("logsink: %s must be a non-empty string", label)
		}
	}
	if e.CausationID != nil && *e.CausationID == e.EventID {
		return errors.New("logsink: causation_id must not equal event_id")
	}
	return nil
}

func chooseUUID(id uuid.UUID) uuid.UUID {
	if id == uuid.Nil {
		return uuid.New()
	}
	return id
}

func chooseTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func chooseCorrelationID(s string) string {
	s = strings.TrimSpace(s)
	if s != "" {
		return s
	}
	return uuid.NewString()
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

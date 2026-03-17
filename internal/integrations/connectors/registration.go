// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"errors"
	"fmt"
	"strings"
)

// Event-type values that the registration consumer accepts.
const (
	EventTypeRegistered = "connector.registered"
	EventTypeHeartbeat  = "connector.heartbeat"
)

// RegistrationMessage is the JSON payload connectors publish on the
// logs topic exchange to announce or refresh themselves.
//
// Descriptor is optional on heartbeats: when absent the stored value
// is preserved (per kernel's contract). When present it replaces the
// stored descriptor wholesale.
type RegistrationMessage struct {
	EventType  string                `json:"event_type"`
	InstanceID string                `json:"instance_id"`
	Tags       []string              `json:"tags,omitempty"`
	Descriptor *CapabilityDescriptor `json:"descriptor,omitempty"`
}

// Validate enforces the wire contract before the consumer touches
// persistence.
func (m RegistrationMessage) Validate() error {
	switch m.EventType {
	case EventTypeRegistered, EventTypeHeartbeat:
	default:
		return fmt.Errorf("connectors: unknown event_type %q", m.EventType)
	}
	id := strings.TrimSpace(m.InstanceID)
	if id == "" {
		return errors.New("connectors: instance_id must be non-empty")
	}
	if len(id) > 255 {
		return errors.New("connectors: instance_id must be at most 255 characters")
	}
	for _, t := range m.Tags {
		if strings.TrimSpace(t) == "" {
			return errors.New("connectors: tags must not contain empty entries")
		}
	}
	return nil
}

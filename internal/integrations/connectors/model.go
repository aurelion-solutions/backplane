// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package connectors owns the ConnectorInstance registry and the
// transport / selection primitives that engines use to reach those
// instances.
//
// Three concerns living here:
//
//   - Registry: ConnectorInstance rows in Postgres, kept current by the
//     registration consumer (self-registration via MQ heartbeat).
//   - Selection: pure tag-set matching against the live registry.
//   - Transport: a connector-specific wrapper around core/rabbitmq.RPCClient
//     that speaks the connector protocol (command body, status, result
//     storage refs).
package connectors

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// onlineThreshold is the cutoff for "still alive" — same 2-minute
// window kernel uses for ConnectorInstance.is_online.
const onlineThreshold = 2 * time.Minute

// ConnectorInstance is one self-registered backend that Aurelion can
// dispatch commands to. JSON tags match the kernel wire contract.
type ConnectorInstance struct {
	bun.BaseModel `bun:"table:connector_instances,alias:ci"`

	ID         uuid.UUID      `bun:"id,pk,type:uuid"           json:"id"`
	InstanceID string         `bun:"instance_id,notnull"       json:"instance_id"`
	Tags       []string       `bun:"tags,type:jsonb,notnull"   json:"tags"`
	Descriptor map[string]any `bun:"descriptor,type:jsonb"     json:"-"`
	LastSeenAt time.Time      `bun:"last_seen_at,notnull"      json:"last_seen_at"`
	CreatedAt  time.Time      `bun:"created_at,notnull"        json:"created_at"`
	UpdatedAt  time.Time      `bun:"updated_at,notnull"        json:"updated_at"`
}

// IsOnline reports whether the instance has been seen recently enough
// to be considered an eligible dispatch target. Pure function of
// LastSeenAt and the current wall clock.
func (c *ConnectorInstance) IsOnline() bool {
	return c.IsOnlineAt(time.Now().UTC())
}

// IsOnlineAt is the deterministic form of IsOnline — tests pass a
// fixed reference time.
func (c *ConnectorInstance) IsOnlineAt(now time.Time) bool {
	return !c.LastSeenAt.Before(now.Add(-onlineThreshold))
}

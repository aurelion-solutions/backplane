// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package rabbitmq builds an *amqp091.Connection and declares the
// durable exchanges referenced in Config. Each exchange carries its
// own type (topic, direct, fanout, headers) — declaration with the
// wrong type fails with PRECONDITION_FAILED, so the caller decides
// per-exchange explicitly.
//
// This package depends on nothing inside backplane (no config, no
// domain). The caller composes a Config and passes it in.
package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Exchange types accepted by AMQP. Provided as named constants so
// callers can write `rabbitmq.Topic` instead of the bare string.
const (
	Topic   = "topic"
	Direct  = "direct"
	Fanout  = "fanout"
	Headers = "headers"
)

// Exchange describes one durable exchange to declare.
type Exchange struct {
	Name string
	Type string // "topic", "direct", "fanout", "headers"
}

// Config holds the inputs New needs.
type Config struct {
	URL       string
	Exchanges []Exchange
}

// Conn wraps an AMQP connection + channel. The single shared channel
// is sufficient for the bootstrap phase — every consumer will open
// its own channel later.
type Conn struct {
	Conn    *amqp.Connection
	Channel *amqp.Channel
}

// Close shuts down channel then connection. Safe to call on a partially
// constructed Conn.
func (c *Conn) Close() error {
	if c == nil {
		return nil
	}
	if c.Channel != nil {
		_ = c.Channel.Close()
	}
	if c.Conn != nil {
		return c.Conn.Close()
	}
	return nil
}

// New dials RabbitMQ, opens one channel, declares every durable
// exchange listed in cfg.Exchanges, and returns the wrapped handle.
// Empty Name or Type entries are skipped.
//
// Callers own the returned *Conn — Close it during shutdown.
func New(cfg Config) (*Conn, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq: open channel: %w", err)
	}

	for _, e := range cfg.Exchanges {
		if e.Name == "" || e.Type == "" {
			continue
		}
		if err := ch.ExchangeDeclare(e.Name, e.Type, true, false, false, false, nil); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return nil, fmt.Errorf("rabbitmq: declare exchange %q (type %s): %w", e.Name, e.Type, err)
		}
	}

	return &Conn{Conn: conn, Channel: ch}, nil
}

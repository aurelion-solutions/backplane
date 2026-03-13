// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package events

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MQ publishes Envelopes to a RabbitMQ topic exchange. Routing key is
// the Envelope.EventType byte-for-byte (the constructor already
// enforces the "<domain>.<entity>.<operation>" grammar). The exchange
// is declared by the core/rabbitmq factory at startup — this sink
// only publishes.
type MQ struct {
	channel  *amqp.Channel
	exchange string
}

// NewMQ returns a sink that publishes to exchange via channel.
// channel must remain open for the sink's lifetime.
func NewMQ(channel *amqp.Channel, exchange string) *MQ {
	return &MQ{channel: channel, exchange: exchange}
}

// Emit implements Sink.
func (m *MQ) Emit(ctx context.Context, e Envelope) error {
	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("events/mq: marshal: %w", err)
	}
	return m.channel.PublishWithContext(ctx,
		m.exchange,
		e.EventType,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}


// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MQSink publishes Events to a RabbitMQ topic exchange. Routing key
// is "{sanitised-component}.{level}". The exchange is declared by
// the core/rabbitmq factory at startup — this sink only publishes.
type MQSink struct {
	channel  *amqp.Channel
	exchange string
}

// NewMQSink returns a sink that publishes to exchange via channel.
// channel must remain open for the sink's lifetime.
func NewMQSink(channel *amqp.Channel, exchange string) *MQSink {
	return &MQSink{channel: channel, exchange: exchange}
}

// Emit implements Sink.
func (s *MQSink) Emit(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("logsink/mq: marshal: %w", err)
	}
	rk := fmt.Sprintf("%s.%s", sanitiseComponent(event.Component), event.Level)
	return s.channel.PublishWithContext(ctx,
		s.exchange,
		rk,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}

func sanitiseComponent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	return s
}

// RegisterMQ wires the "mq" provider. Must be called after the
// rabbitmq.Conn is established because it needs a live *amqp.Channel.
func RegisterMQ(f *Factory, channel *amqp.Channel, exchange string) {
	f.Register("mq", func() (Sink, error) {
		return NewMQSink(channel, exchange), nil
	})
}

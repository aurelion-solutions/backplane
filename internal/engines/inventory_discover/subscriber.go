// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_discover

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

// DefaultSubscriberQueue is the queue used when SubscriberConfig.Queue
// is left empty.
const DefaultSubscriberQueue = "aurelion.discover.connector_events"

// SubscriberConfig wires the lifecycle-event listener.
type SubscriberConfig struct {
	Channel        *amqp.Channel
	EventsExchange string
	Queue          string
	Service        *Service
	Log            *slog.Logger
}

// RunSubscriber consumes connector.discover.* events from the platform
// events exchange and routes them into Service.HandleConnectorEvent.
// Blocks until ctx is cancelled.
func RunSubscriber(ctx context.Context, cfg SubscriberConfig) error {
	queue := cfg.Queue
	if queue == "" {
		queue = DefaultSubscriberQueue
	}
	log := cfg.Log
	return rabbitmq.Consume(ctx, cfg.Channel, rabbitmq.ConsumeConfig{
		Exchange: cfg.EventsExchange,
		Queue:    queue,
		BindingKeys: []string{
			EventConnectorStarted,
			EventConnectorCompleted,
			EventConnectorFailed,
		},
		Handler: func(ctx context.Context, body []byte) error {
			handleConnectorEvent(ctx, cfg.Service, log, body)
			return nil
		},
	})
}

func handleConnectorEvent(ctx context.Context, svc *Service, log *slog.Logger, body []byte) {
	var env events.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		log.Warn("discover subscriber: bad envelope", slog.Any("err", err))
		return
	}
	if env.CorrelationID == "" {
		log.Warn("discover subscriber: missing correlation_id",
			slog.String("event_type", env.EventType))
		return
	}
	errMsg := ""
	if env.Payload != nil {
		if v, ok := env.Payload["error"].(string); ok {
			errMsg = v
		}
	}
	if err := svc.HandleConnectorEvent(ctx, env.CorrelationID, env.EventType, errMsg); err != nil {
		log.Error("discover subscriber: handle failed",
			slog.String("correlation_id", env.CorrelationID),
			slog.String("event_type", env.EventType),
			slog.Any("err", err),
		)
	}
}

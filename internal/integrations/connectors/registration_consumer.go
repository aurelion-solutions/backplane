// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RegistrationConsumerConfig wires the consumer to the topic exchange
// that carries connector.registered / connector.heartbeat messages.
type RegistrationConsumerConfig struct {
	// Channel is the AMQP channel the consumer subscribes on. The
	// composition root owns its lifecycle; the consumer never closes it.
	Channel *amqp.Channel

	// Exchange is the durable topic exchange carrying registration
	// traffic (typically aurelion.logs or a dedicated registration
	// exchange).
	Exchange string

	// Queue is the durable queue this consumer subscribes to. Multiple
	// backplane instances should share the same queue name so the
	// broker fans registrations to exactly one of them.
	Queue string

	// BindingKeys filters which routing keys the queue receives. Pass
	// the wildcards covering connector.registered and connector.heartbeat
	// (e.g. "connector.*").
	BindingKeys []string
}

// RunRegistrationConsumer blocks until ctx is cancelled, dispatching
// each incoming message to svc.RegisterFromMessage. Malformed messages
// are logged and dropped (no requeue) so a poison pill cannot stall
// the queue.
func RunRegistrationConsumer(
	ctx context.Context,
	log *slog.Logger,
	svc *Service,
	cfg RegistrationConsumerConfig,
) error {
	if cfg.Channel == nil {
		return fmt.Errorf("connectors/registration_consumer: nil channel")
	}
	if cfg.Exchange == "" || cfg.Queue == "" {
		return fmt.Errorf("connectors/registration_consumer: exchange and queue required")
	}
	keys := cfg.BindingKeys
	if len(keys) == 0 {
		keys = []string{"connector.*"}
	}

	handler := func(hctx context.Context, body []byte) error {
		var msg RegistrationMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			log.Warn("connector registration: malformed payload",
				slog.String("err", err.Error()),
			)
			return nil // drop, do not requeue
		}
		inst, err := svc.RegisterFromMessage(hctx, msg)
		if err != nil {
			log.Warn("connector registration: rejected",
				slog.String("err", err.Error()),
				slog.String("instance_id", msg.InstanceID),
			)
			return nil // drop validation failures, do not requeue
		}
		log.Info("connector registered",
			slog.String("event_type", msg.EventType),
			slog.String("instance_id", inst.InstanceID),
			slog.Any("tags", inst.Tags),
		)
		return nil
	}

	return rabbitmq.Consume(ctx, cfg.Channel, rabbitmq.ConsumeConfig{
		Exchange:    cfg.Exchange,
		Queue:       cfg.Queue,
		BindingKeys: keys,
		Handler:     handler,
	})
}

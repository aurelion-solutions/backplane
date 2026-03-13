// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package rabbitmq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConsumeConfig describes how to bind a durable consumer queue to an
// existing topic exchange and dispatch each delivery to a handler.
type ConsumeConfig struct {
	Exchange    string   // existing topic exchange (declared by rabbitmq.New)
	Queue       string   // durable queue to declare and consume from
	BindingKeys []string // routing-key patterns ("#" = catch-all)
	Handler     func(ctx context.Context, body []byte) error
}

// Consume declares the queue, binds it to exchange for every key,
// and runs the dispatch loop until ctx is cancelled.
//
// Handler errors trigger basic_nack with requeue=false (dead-letter
// territory); a clean return triggers basic_ack. Delivery is fired
// per-message, sequentially, in the calling goroutine.
func Consume(ctx context.Context, channel *amqp.Channel, cfg ConsumeConfig) error {
	if _, err := channel.QueueDeclare(cfg.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq/consumer: declare queue %q: %w", cfg.Queue, err)
	}
	keys := cfg.BindingKeys
	if len(keys) == 0 {
		keys = []string{"#"}
	}
	for _, k := range keys {
		if err := channel.QueueBind(cfg.Queue, k, cfg.Exchange, false, nil); err != nil {
			return fmt.Errorf("rabbitmq/consumer: bind %q -> %q (%q): %w", cfg.Queue, cfg.Exchange, k, err)
		}
	}

	deliveries, err := channel.Consume(cfg.Queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq/consumer: consume: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("rabbitmq/consumer: delivery channel closed")
			}
			if err := cfg.Handler(ctx, d.Body); err != nil {
				_ = d.Nack(false, false)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

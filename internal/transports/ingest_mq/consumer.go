// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package ingest_mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/engines/inventory_ingest"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Default ingest exchange + queue. Topic exchange so a future
// per-dataset consumer fan-out is just an extra binding.
const (
	DefaultExchange = "aurelion.ingest"
	DefaultQueue    = "aurelion.ingest.records"
)

// FlushSize is the per-bucket record cap that triggers a Process
// call before FlushInterval elapses.
const FlushSize = 10_000

// FlushInterval is the wall-clock cap a bucket can sit idle before
// it is flushed.
const FlushInterval = 5 * time.Second

// Prefetch is the AMQP basic.qos prefetch_count. Larger than
// FlushSize so a consumer can build a full bucket without
// back-pressuring the connector.
const Prefetch = 20_000

// HeaderSource and HeaderCorrelationID are the AMQP message headers
// the connector must stamp on every published record. Routing key is
// dataset_type.
const (
	HeaderSource        = "source"
	HeaderCorrelationID = "correlation_id"
)

// Config wires the consumer.
type Config struct {
	Channel  *amqp.Channel
	Exchange string
	Queue    string
	Service  *inventory_ingest.Service
	Log      *slog.Logger
}

// Run blocks until ctx is cancelled, consuming records from the
// ingest exchange and flushing them through inventory_ingest.Process
// in (source, dataset_type, correlation_id) buckets.
func Run(ctx context.Context, cfg Config) error {
	exchange := cfg.Exchange
	if exchange == "" {
		exchange = DefaultExchange
	}
	queue := cfg.Queue
	if queue == "" {
		queue = DefaultQueue
	}
	log := cfg.Log

	if _, err := cfg.Channel.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("ingest_mq: declare queue: %w", err)
	}
	if err := cfg.Channel.QueueBind(queue, "#", exchange, false, nil); err != nil {
		return fmt.Errorf("ingest_mq: bind queue: %w", err)
	}
	if err := cfg.Channel.Qos(Prefetch, 0, false); err != nil {
		return fmt.Errorf("ingest_mq: qos: %w", err)
	}
	deliveries, err := cfg.Channel.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("ingest_mq: consume: %w", err)
	}

	state := newState(cfg.Service, log)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			state.flushAll(ctx)
			return nil
		case d, ok := <-deliveries:
			if !ok {
				state.flushAll(ctx)
				return fmt.Errorf("ingest_mq: delivery channel closed")
			}
			state.append(ctx, d)
		case <-ticker.C:
			state.flushExpired(ctx)
		}
	}
}

type bucketKey struct {
	source        string
	datasetType   string
	correlationID string
}

type bucket struct {
	records      []map[string]any
	deliveryTags []uint64
	firstSeenAt  time.Time
}

type state struct {
	mu      sync.Mutex
	svc     *inventory_ingest.Service
	log     *slog.Logger
	buckets map[bucketKey]*bucket
	channel *amqp.Channel
}

func newState(svc *inventory_ingest.Service, log *slog.Logger) *state {
	return &state{
		svc:     svc,
		log:     log,
		buckets: map[bucketKey]*bucket{},
	}
}

// append parses a single delivery and adds it to the matching bucket.
// On malformed messages we nack with requeue=false (DLQ territory)
// rather than retry-loop on garbage.
func (s *state) append(ctx context.Context, d amqp.Delivery) {
	source, _ := headerString(d, HeaderSource)
	correlationID, _ := headerString(d, HeaderCorrelationID)
	datasetType := d.RoutingKey
	if source == "" || datasetType == "" {
		s.log.Warn("ingest_mq: dropping message with missing source / dataset_type",
			slog.String("routing_key", datasetType), slog.String("source", source))
		_ = d.Nack(false, false)
		return
	}
	var rec map[string]any
	if err := json.Unmarshal(d.Body, &rec); err != nil {
		s.log.Warn("ingest_mq: dropping malformed record",
			slog.Any("err", err), slog.String("dataset_type", datasetType))
		_ = d.Nack(false, false)
		return
	}

	key := bucketKey{source: source, datasetType: datasetType, correlationID: correlationID}

	s.mu.Lock()
	b, ok := s.buckets[key]
	if !ok {
		b = &bucket{firstSeenAt: time.Now().UTC()}
		s.buckets[key] = b
	}
	b.records = append(b.records, rec)
	b.deliveryTags = append(b.deliveryTags, d.DeliveryTag)
	s.channel = d.Acknowledger.(*amqp.Channel)
	full := len(b.records) >= FlushSize
	s.mu.Unlock()

	if full {
		s.flushKey(ctx, key)
	}
}

func (s *state) flushExpired(ctx context.Context) {
	now := time.Now().UTC()
	s.mu.Lock()
	expired := make([]bucketKey, 0, len(s.buckets))
	for k, b := range s.buckets {
		if now.Sub(b.firstSeenAt) >= FlushInterval {
			expired = append(expired, k)
		}
	}
	s.mu.Unlock()
	for _, k := range expired {
		s.flushKey(ctx, k)
	}
}

func (s *state) flushAll(ctx context.Context) {
	s.mu.Lock()
	keys := make([]bucketKey, 0, len(s.buckets))
	for k := range s.buckets {
		keys = append(keys, k)
	}
	s.mu.Unlock()
	for _, k := range keys {
		s.flushKey(ctx, k)
	}
}

func (s *state) flushKey(ctx context.Context, key bucketKey) {
	s.mu.Lock()
	b, ok := s.buckets[key]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.buckets, key)
	channel := s.channel
	s.mu.Unlock()

	if len(b.records) == 0 {
		return
	}

	flushCtx := ctx
	if key.correlationID != "" {
		flushCtx = correlation.WithID(ctx, key.correlationID)
	}

	result, err := s.svc.Process(flushCtx, inventory_ingest.Request{
		Source:        key.source,
		DatasetType:   key.datasetType,
		CorrelationID: key.correlationID,
		Records:       b.records,
	})
	if err != nil {
		// Validation errors are not retriable, lake-write failures
		// might be — but our DuckDB anti-join is idempotent, so a
		// requeue would just retry forever. Ack and log loudly.
		s.log.Error("ingest_mq: process failed",
			slog.Any("err", err),
			slog.String("source", key.source),
			slog.String("dataset_type", key.datasetType),
			slog.String("correlation_id", key.correlationID),
			slog.Int("records", len(b.records)),
		)
	} else {
		s.log.Info("ingest_mq: flushed",
			slog.String("source", result.Source),
			slog.String("dataset_type", result.DatasetType),
			slog.String("correlation_id", result.CorrelationID),
			slog.Int("received", result.Received),
			slog.Int("written", result.Written),
			slog.Int("skipped", result.Skipped),
		)
	}

	// Acks always go through, regardless of Process outcome.
	for _, tag := range b.deliveryTags {
		if err := channel.Ack(tag, false); err != nil {
			if !errors.Is(err, amqp.ErrClosed) {
				s.log.Warn("ingest_mq: ack failed", slog.Any("err", err), slog.Uint64("delivery_tag", tag))
			}
		}
	}
}

func headerString(d amqp.Delivery, key string) (string, bool) {
	if d.Headers == nil {
		return "", false
	}
	v, ok := d.Headers[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

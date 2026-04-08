// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package matcher

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/uptrace/bun"
)

// AdvisoryLockKey is the 64-bit integer "AURELMAT" used as the
// session-level pg_advisory_lock key. Only one matcher across the
// cluster holds it at a time.
const AdvisoryLockKey int64 = 0x4155_5245_4C4D_4154

// StandbyRetryInterval is how often a standby retries acquiring the
// advisory lock when the active matcher holds it.
const StandbyRetryInterval = 1 * time.Second

// Catalog is the runtime contract the matcher uses to look up MQ
// triggers on each delivery.
type Catalog interface {
	All() []*loader.Definition
}

// Matcher consumes events from RabbitMQ and drives the two effects
// described in the package doc.
type Matcher struct {
	db       *bun.DB
	channel  *amqp.Channel
	svc      *orchestrator.Service
	catalog  Catalog
	log      *slog.Logger
	exchange string
	queue    string
}

// Config is the constructor parameter struct.
type Config struct {
	DB             *bun.DB
	Channel        *amqp.Channel
	Service        *orchestrator.Service
	Catalog        Catalog
	Log            *slog.Logger
	EventsExchange string
	MatcherQueue   string
}

// New composes a Matcher.
func New(c Config) *Matcher {
	return &Matcher{
		db:       c.DB,
		channel:  c.Channel,
		svc:      c.Service,
		catalog:  c.Catalog,
		log:      c.Log,
		exchange: c.EventsExchange,
		queue:    c.MatcherQueue,
	}
}

// Loop runs until ctx is cancelled. Acquires the advisory lock on a
// dedicated connection (held for the lifetime of the active matcher),
// then drives the MQ consume loop. Standbys retry every
// StandbyRetryInterval.
func (m *Matcher) Loop(ctx context.Context) error {
	m.log.Info("matcher loop starting")
	for {
		select {
		case <-ctx.Done():
			m.log.Info("matcher loop stopping")
			return nil
		default:
		}
		acquired, conn, err := m.tryAcquireLock(ctx)
		if err != nil {
			m.log.Warn("matcher advisory lock acquire failed", slog.Any("err", err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(StandbyRetryInterval):
			}
			continue
		}
		if !acquired {
			m.log.Debug("matcher standby (lock held elsewhere)")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(StandbyRetryInterval):
			}
			continue
		}
		m.log.Info("matcher acquired advisory lock — consuming events")
		consumeErr := m.consume(ctx)
		_, _ = conn.ExecContext(context.Background(),
			"SELECT pg_advisory_unlock(?)", AdvisoryLockKey)
		_ = conn.Close()
		if consumeErr != nil && !errors.Is(consumeErr, context.Canceled) {
			m.log.Warn("matcher consume terminated — re-acquiring", slog.Any("err", consumeErr))
		}
	}
}

// tryAcquireLock opens a dedicated *sql.Conn and runs pg_try_advisory_lock.
// The returned conn is held by the caller and closed on shutdown so
// the session-level lock is released.
func (m *Matcher) tryAcquireLock(ctx context.Context) (bool, *sql.Conn, error) {
	conn, err := m.db.DB.Conn(ctx)
	if err != nil {
		return false, nil, err
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx,
		"SELECT pg_try_advisory_lock($1)", AdvisoryLockKey).Scan(&acquired); err != nil {
		_ = conn.Close()
		return false, nil, err
	}
	if !acquired {
		_ = conn.Close()
		return false, nil, nil
	}
	return true, conn, nil
}

// consume drives the consume loop until ctx is cancelled or the
// channel breaks.
func (m *Matcher) consume(ctx context.Context) error {
	return rabbitmq.Consume(ctx, m.channel, rabbitmq.ConsumeConfig{
		Exchange:    m.exchange,
		Queue:       m.queue,
		BindingKeys: []string{"#"},
		Handler: func(ctx context.Context, body []byte) error {
			m.handleDelivery(ctx, body)
			// Always ack: there is no useful retry semantic for a
			// poison message at this layer. (a) is best-effort; (b)
			// hits idempotency dedupe on the partial UNIQUE so a
			// re-delivered event won't double-fire.
			return nil
		},
	})
}

func (m *Matcher) handleDelivery(ctx context.Context, body []byte) {
	var envelope events.Envelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		m.log.Warn("matcher poison message — failed to unmarshal envelope",
			slog.Any("err", err), slog.Int("body_len", len(body)))
		return
	}
	if envelope.EventType == "" {
		return
	}
	payload := envelope.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	// Effect (a) — independent transaction per resolved waiter.
	m.resolveWaiters(ctx, envelope.EventType, payload)
	// Effect (b) — independent transaction per fired trigger.
	m.fireMQTriggers(ctx, envelope.EventType, payload)
}

func (m *Matcher) resolveWaiters(ctx context.Context, eventType string, payload map[string]any) {
	// Read step ids that match the event in its own short Tx so the
	// resolve transactions below don't share its snapshot.
	var stepIDs []uuid.UUID
	err := m.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		ids, err := m.svc.FindMatchingWaiterStepIDs(ctx, tx, eventType, payload)
		stepIDs = ids
		return err
	})
	if err != nil {
		m.log.Warn("matcher find waiters failed", slog.Any("err", err))
		return
	}
	for _, stepID := range stepIDs {
		err := m.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			ok, err := m.svc.ResolveEventWaiter(ctx, tx, stepID, payload)
			if err != nil {
				return err
			}
			if ok {
				m.log.Info("matcher resolved waiter",
					slog.String("step_run_id", stepID.String()),
					slog.String("event_type", eventType))
			}
			return nil
		})
		if err != nil {
			m.log.Warn("matcher resolve waiter failed",
				slog.String("step_run_id", stepID.String()), slog.Any("err", err))
		}
	}
}

func (m *Matcher) fireMQTriggers(ctx context.Context, eventType string, payload map[string]any) {
	hits := FindMatchingMQTriggers(m.catalog.All(), eventType, payload)
	for _, hit := range hits {
		args := map[string]any{}
		if a, ok := hit.Trigger["args"].(map[string]any); ok {
			for k, v := range a {
				args[k] = v
			}
		}
		if afp, ok := hit.Trigger["args_from_payload"].(map[string]any); ok {
			for k, v := range ExtractArgsFromPayload(afp, payload) {
				args[k] = v
			}
		}
		err := m.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			res, err := m.svc.CreateRun(ctx, tx, orchestrator.CreateRunInput{
				PipelineName:    hit.Definition.Name,
				PipelineVersion: hit.Definition.Version,
				Args:            args,
				TriggerSource:   orchestrator.TriggerMQ,
			})
			if err != nil {
				return err
			}
			if res.Created {
				m.log.Info("matcher fired MQ trigger",
					slog.String("pipeline", hit.Definition.Name),
					slog.String("run_id", res.Run.ID.String()),
					slog.String("event_type", eventType))
			}
			return nil
		})
		if err != nil {
			m.log.Warn("matcher fire MQ trigger failed",
				slog.String("pipeline", hit.Definition.Name), slog.Any("err", err))
		}
	}
}

// keep fmt used by the panic-prone Sprintf paths (no-op compile shim
// in case future edits remove the only fmt user).
var _ = fmt.Sprintf

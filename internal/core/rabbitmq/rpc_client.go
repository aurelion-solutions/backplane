// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RPCClientConfig configures the generic request/reply client.
//
// The client opens its own AMQP channel on a caller-provided
// *amqp.Connection (consume + publish on the same dedicated channel
// avoids interference with registration consumers and event publishers
// that share the connection).
type RPCClientConfig struct {
	// ResponsesExchange is the durable direct exchange where remote
	// peers publish reply messages. The client binds its private reply
	// queue to this exchange by ClientID.
	ResponsesExchange string

	// Timeout caps how long Request waits for a reply. Zero means
	// 60 seconds (parity with kernel's AsyncRabbitMQRPCClient).
	Timeout time.Duration

	// ClientID identifies this client on the responses exchange. If
	// empty a fresh UUID v4 is generated at NewRPCClient time.
	ClientID string
}

// RPCRequest is one outgoing request.
//
// Body is opaque to the transport — the caller (a connector-specific
// wrapper, an LLM proxy, etc.) decides the wire shape. The reply target
// the remote peer must echo back lives in ReplyExchange / ReplyRoutingKey
// available via RPCClient.ReplyTarget, so the caller can encode them
// in Body if the protocol requires it (kernel's connector protocol does).
type RPCRequest struct {
	Exchange      string
	RoutingKey    string
	CorrelationID string // if empty a fresh UUID v4 is generated
	Body          []byte
}

// RPCClient is a generic AMQP request/reply client. It does not know
// about connectors or any specific protocol — it just correlates
// outgoing publishes with replies that carry the same correlation_id.
//
// Lifecycle: NewRPCClient -> Start -> Request (many) -> Close.
type RPCClient struct {
	conn              *amqp.Connection
	responsesExchange string
	clientID          string
	timeout           time.Duration
	replyQueueName    string

	mu      sync.Mutex
	channel *amqp.Channel
	pending map[string]chan rpcReply
	started bool
	closed  bool

	stop chan struct{}
	done chan struct{}
}

type rpcReply struct {
	body []byte
	err  error
}

// NewRPCClient constructs a client bound to conn. Start must be called
// before Request. The caller owns the *amqp.Connection and is
// responsible for its lifetime.
func NewRPCClient(conn *amqp.Connection, cfg RPCClientConfig) *RPCClient {
	id := cfg.ClientID
	if id == "" {
		id = uuid.NewString()
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &RPCClient{
		conn:              conn,
		responsesExchange: cfg.ResponsesExchange,
		clientID:          id,
		timeout:           timeout,
		replyQueueName:    fmt.Sprintf("aurelion.api.rpc.%s.replies", id),
		pending:           make(map[string]chan rpcReply),
		stop:              make(chan struct{}),
		done:              make(chan struct{}),
	}
}

// ClientID is this client's identifier on the responses exchange.
// Remote peers should publish replies with routing_key == ClientID.
func (c *RPCClient) ClientID() string {
	return c.clientID
}

// ReplyTarget reports where remote peers should publish reply messages
// so this client can route them back to the waiting Request. Encode
// these into the request Body if your protocol requires it.
func (c *RPCClient) ReplyTarget() (exchange string, routingKey string) {
	return c.responsesExchange, c.clientID
}

// Start opens a dedicated channel, declares the reply queue
// (exclusive, auto-delete, non-durable), binds it to the responses
// exchange by ClientID, and starts dispatching replies. Call exactly
// once. Returns an error if the channel / topology setup fails.
func (c *RPCClient) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return errors.New("rabbitmq/rpc_client: already started")
	}
	if c.closed {
		c.mu.Unlock()
		return errors.New("rabbitmq/rpc_client: client is closed")
	}
	c.mu.Unlock()

	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq/rpc_client: open channel: %w", err)
	}

	if err := ch.ExchangeDeclare(c.responsesExchange, Direct, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq/rpc_client: declare responses exchange %q: %w", c.responsesExchange, err)
	}

	if _, err := ch.QueueDeclare(c.replyQueueName, false, true, true, false, nil); err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq/rpc_client: declare reply queue %q: %w", c.replyQueueName, err)
	}

	if err := ch.QueueBind(c.replyQueueName, c.clientID, c.responsesExchange, false, nil); err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq/rpc_client: bind reply queue: %w", err)
	}

	deliveries, err := ch.Consume(c.replyQueueName, "", true, true, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq/rpc_client: consume reply queue: %w", err)
	}

	c.mu.Lock()
	c.channel = ch
	c.started = true
	c.mu.Unlock()

	go c.dispatchLoop(deliveries)
	_ = ctx // reserved for future cancellation semantics on Start itself
	return nil
}

// Close cancels every pending request, stops the dispatch loop, and
// closes the dedicated channel. The underlying connection stays open —
// the caller owns it.
func (c *RPCClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	pending := c.pending
	c.pending = nil
	ch := c.channel
	c.channel = nil
	started := c.started
	c.mu.Unlock()

	if started {
		close(c.stop)
	}

	for _, waiter := range pending {
		select {
		case waiter <- rpcReply{err: errors.New("rabbitmq/rpc_client: closed")}:
		default:
		}
	}

	if ch != nil {
		_ = ch.Close()
	}
	if started {
		<-c.done
	}
	return nil
}

// Request publishes req on its exchange/routing-key and waits for the
// matching reply (correlated by correlation_id). Returns the raw reply
// body or an error (timeout, transport failure, client closed).
//
// CorrelationID propagation: if req.CorrelationID is set the caller's
// id wins; otherwise a UUID v4 is generated here. Caller-supplied ids
// support trace continuity (HTTP X-Correlation-ID -> service ->
// connector RPC -> response event all stamped with the same id).
func (c *RPCClient) Request(ctx context.Context, req RPCRequest) ([]byte, error) {
	c.mu.Lock()
	if !c.started || c.closed {
		c.mu.Unlock()
		return nil, errors.New("rabbitmq/rpc_client: not started or already closed")
	}
	ch := c.channel
	c.mu.Unlock()

	cid := req.CorrelationID
	if cid == "" {
		cid = uuid.NewString()
	}

	waiter := make(chan rpcReply, 1)

	c.mu.Lock()
	if c.pending == nil {
		c.mu.Unlock()
		return nil, errors.New("rabbitmq/rpc_client: client closed")
	}
	if _, exists := c.pending[cid]; exists {
		c.mu.Unlock()
		return nil, fmt.Errorf("rabbitmq/rpc_client: correlation_id %q already in-flight", cid)
	}
	c.pending[cid] = waiter
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		if c.pending != nil {
			delete(c.pending, cid)
		}
		c.mu.Unlock()
	}()

	msg := amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		CorrelationId: cid,
		MessageId:     uuid.NewString(),
		ReplyTo:       c.clientID,
		Body:          req.Body,
	}
	if err := ch.PublishWithContext(ctx, req.Exchange, req.RoutingKey, false, false, msg); err != nil {
		return nil, fmt.Errorf("rabbitmq/rpc_client: publish: %w", err)
	}

	timer := time.NewTimer(c.timeout)
	defer timer.Stop()

	select {
	case rep := <-waiter:
		if rep.err != nil {
			return nil, rep.err
		}
		return rep.body, nil
	case <-timer.C:
		return nil, fmt.Errorf("rabbitmq/rpc_client: timeout after %s (correlation_id=%s)", c.timeout, cid)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *RPCClient) dispatchLoop(deliveries <-chan amqp.Delivery) {
	defer close(c.done)
	for {
		select {
		case <-c.stop:
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			if d.CorrelationId == "" {
				continue
			}
			c.mu.Lock()
			waiter, found := c.pending[d.CorrelationId]
			c.mu.Unlock()
			if !found {
				continue
			}
			select {
			case waiter <- rpcReply{body: d.Body}:
			default:
			}
		}
	}
}

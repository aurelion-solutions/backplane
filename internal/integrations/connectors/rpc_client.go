// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/google/uuid"
)

// Transport is the slice of core/rabbitmq.RPCClient this package needs.
// Defined as an interface so tests can substitute a fake without
// dragging in an AMQP channel.
type Transport interface {
	ReplyTarget() (exchange, routingKey string)
	Request(ctx context.Context, req rabbitmq.RPCRequest) ([]byte, error)
}

// LakeReader reads a result batch the remote connector placed in the
// data lake instead of returning it inline. Implementations live in
// platform/storage; the package consumer injects one.
type LakeReader interface {
	ReadBatch(ctx context.Context, provider string, storageKey string) ([]map[string]any, error)
}

// RPCClient speaks the connector RPC protocol on top of the generic
// AMQP request/reply primitive. One instance per backplane process.
type RPCClient struct {
	transport        Transport
	lake             LakeReader
	commandsExchange string
}

// NewRPCClient wires the connector-specific protocol to a generic
// transport. lake may be nil only when the caller guarantees no
// connector will reply with a result_storage_ref.
func NewRPCClient(transport Transport, lake LakeReader, commandsExchange string) *RPCClient {
	return &RPCClient{
		transport:        transport,
		lake:             lake,
		commandsExchange: commandsExchange,
	}
}

// TraceContext optionally pins one RPC call into a larger event chain.
// The remote connector echoes these fields back when it reports the
// result event so causation links survive the round-trip.
type TraceContext struct {
	ParentEventID *uuid.UUID
	InitiatorKind string
	InitiatorID   string
	TargetKind    string
	TargetID      string
}

// InvokeRequest is the high-level call the caller composes.
//
// CorrelationID resolution chain (mirrors kernel's contract):
//
//  1. Explicit field wins.
//  2. Otherwise correlation.ID(ctx) wins.
//  3. Otherwise the underlying transport generates a UUID v4.
type InvokeRequest struct {
	InstanceID             string
	Operation              string
	Payload                map[string]any
	ResultStorageRequested bool
	CorrelationID          string
	Trace                  *TraceContext
}

// InvokeResult is the unwrapped reply.
//
// Records is populated only when the remote connector responded with a
// result_storage_ref AND a LakeReader is configured; otherwise Payload
// carries the inline reply verbatim.
type InvokeResult struct {
	Status  string
	Payload map[string]any
	Records []map[string]any
}

// Invoke publishes a command and awaits the reply.
//
// Errors:
//   - context cancellation / RPC timeout — wrapped from the transport.
//   - JSON decode failure on the reply.
//   - non-ok status — returned as *ErrRPCStatus carrying status + message.
func (c *RPCClient) Invoke(ctx context.Context, req InvokeRequest) (*InvokeResult, error) {
	if req.InstanceID == "" {
		return nil, fmt.Errorf("connectors/rpc: instance_id required")
	}
	if req.Operation == "" {
		return nil, fmt.Errorf("connectors/rpc: operation required")
	}

	cid := req.CorrelationID
	if cid == "" {
		if v, ok := correlation.ID(ctx); ok {
			cid = v
		}
	}

	replyExch, replyRK := c.transport.ReplyTarget()
	body := map[string]any{
		"correlation_id":           cid,
		"reply_exchange":           replyExch,
		"reply_routing_key":        replyRK,
		"operation":                req.Operation,
		"result_storage_requested": req.ResultStorageRequested,
		"payload":                  req.Payload,
	}
	if req.Trace != nil {
		if req.Trace.ParentEventID != nil {
			body["trace_parent_event_id"] = req.Trace.ParentEventID.String()
		}
		body["trace_initiator_type"] = req.Trace.InitiatorKind
		body["trace_initiator_id"] = req.Trace.InitiatorID
		body["trace_target_type"] = req.Trace.TargetKind
		body["trace_target_id"] = req.Trace.TargetID
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("connectors/rpc: encode body: %w", err)
	}

	replyBytes, err := c.transport.Request(ctx, rabbitmq.RPCRequest{
		Exchange:      c.commandsExchange,
		RoutingKey:    req.InstanceID,
		CorrelationID: cid,
		Body:          encoded,
	})
	if err != nil {
		return nil, fmt.Errorf("connectors/rpc: invoke %s on %s: %w", req.Operation, req.InstanceID, err)
	}

	return c.decodeReply(ctx, replyBytes)
}

func (c *RPCClient) decodeReply(ctx context.Context, raw []byte) (*InvokeResult, error) {
	var envelope struct {
		Status           string          `json:"status"`
		Error            json.RawMessage `json:"error,omitempty"`
		Payload          json.RawMessage `json:"payload,omitempty"`
		ResultStorageRef *struct {
			Provider   string `json:"provider"`
			StorageKey string `json:"storage_key"`
		} `json:"result_storage_ref,omitempty"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("connectors/rpc: decode reply: %w", err)
	}

	status := envelope.Status
	if status == "" {
		status = "ok"
	}
	if status != "ok" {
		return nil, &ErrRPCStatus{Status: status, Message: extractErrorMessage(envelope.Error)}
	}

	result := &InvokeResult{Status: status}

	if envelope.ResultStorageRef != nil {
		if c.lake == nil {
			return nil, fmt.Errorf("connectors/rpc: reply carries result_storage_ref but no LakeReader is configured")
		}
		records, err := c.lake.ReadBatch(ctx, envelope.ResultStorageRef.Provider, envelope.ResultStorageRef.StorageKey)
		if err != nil {
			return nil, fmt.Errorf("connectors/rpc: read result batch: %w", err)
		}
		result.Records = records
		return result, nil
	}

	if len(envelope.Payload) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(envelope.Payload, &payload); err == nil {
			result.Payload = payload
		} else {
			// payload may be a bare list — expose it under "_raw" so
			// callers that expect a map are forced to be explicit.
			var raw any
			if err := json.Unmarshal(envelope.Payload, &raw); err != nil {
				return nil, fmt.Errorf("connectors/rpc: decode payload: %w", err)
			}
			result.Payload = map[string]any{"_raw": raw}
		}
	}
	return result, nil
}

func extractErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "connector request failed"
	}
	var asObject struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil && asObject.Message != "" {
		return asObject.Message
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil && asString != "" {
		return asString
	}
	return "connector request failed"
}

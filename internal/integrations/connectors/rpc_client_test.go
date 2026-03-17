// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
)

type fakeTransport struct {
	lastRequest rabbitmq.RPCRequest
	reply       []byte
	err         error
}

func (f *fakeTransport) ReplyTarget() (string, string) {
	return "aurelion.connectors.responses", "client-xyz"
}

func (f *fakeTransport) Request(ctx context.Context, req rabbitmq.RPCRequest) ([]byte, error) {
	f.lastRequest = req
	if f.err != nil {
		return nil, f.err
	}
	return f.reply, nil
}

type fakeLake struct {
	provider string
	key      string
	records  []map[string]any
	err      error
}

func (f *fakeLake) ReadBatch(_ context.Context, provider, key string) ([]map[string]any, error) {
	f.provider = provider
	f.key = key
	return f.records, f.err
}

func TestInvoke_EmbedsReplyTargetAndCorrelation(t *testing.T) {
	ft := &fakeTransport{reply: []byte(`{"status":"ok","payload":{"account_id":"a-1"}}`)}
	c := NewRPCClient(ft, nil, "aurelion.connectors.commands")
	ctx := correlation.WithID(context.Background(), "cid-trace")

	result, err := c.Invoke(ctx, InvokeRequest{
		InstanceID: "sf-prod-1",
		Operation:  "account_create",
		Payload:    map[string]any{"email": "x@y.z"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "ok" || result.Payload["account_id"] != "a-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if ft.lastRequest.Exchange != "aurelion.connectors.commands" {
		t.Fatalf("want commands exchange, got %q", ft.lastRequest.Exchange)
	}
	if ft.lastRequest.RoutingKey != "sf-prod-1" {
		t.Fatalf("want routing key=instance_id, got %q", ft.lastRequest.RoutingKey)
	}
	if ft.lastRequest.CorrelationID != "cid-trace" {
		t.Fatalf("want correlation_id from ctx, got %q", ft.lastRequest.CorrelationID)
	}

	var body map[string]any
	if err := json.Unmarshal(ft.lastRequest.Body, &body); err != nil {
		t.Fatalf("body must be JSON: %v", err)
	}
	if body["reply_exchange"] != "aurelion.connectors.responses" {
		t.Fatalf("body must carry reply_exchange, got %v", body["reply_exchange"])
	}
	if body["reply_routing_key"] != "client-xyz" {
		t.Fatalf("body must carry reply_routing_key, got %v", body["reply_routing_key"])
	}
	if body["correlation_id"] != "cid-trace" {
		t.Fatalf("body must carry correlation_id, got %v", body["correlation_id"])
	}
}

func TestInvoke_ExplicitCorrelationOverridesContext(t *testing.T) {
	ft := &fakeTransport{reply: []byte(`{"status":"ok","payload":{}}`)}
	c := NewRPCClient(ft, nil, "x")
	ctx := correlation.WithID(context.Background(), "from-ctx")

	if _, err := c.Invoke(ctx, InvokeRequest{
		InstanceID:    "i",
		Operation:     "op",
		CorrelationID: "explicit",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ft.lastRequest.CorrelationID != "explicit" {
		t.Fatalf("want explicit correlation, got %q", ft.lastRequest.CorrelationID)
	}
}

func TestInvoke_NonOkStatus_ReturnsTypedError(t *testing.T) {
	ft := &fakeTransport{reply: []byte(`{"status":"error","error":{"message":"connector said no"}}`)}
	c := NewRPCClient(ft, nil, "x")
	_, err := c.Invoke(context.Background(), InvokeRequest{InstanceID: "i", Operation: "op"})
	var typed *ErrRPCStatus
	if !errors.As(err, &typed) {
		t.Fatalf("want *ErrRPCStatus, got %v", err)
	}
	if typed.Status != "error" || typed.Message != "connector said no" {
		t.Fatalf("unexpected typed error: %+v", typed)
	}
}

func TestInvoke_ResultStorageRef_PullsFromLake(t *testing.T) {
	ft := &fakeTransport{reply: []byte(`{"status":"ok","result_storage_ref":{"provider":"file","storage_key":"k/123"}}`)}
	lake := &fakeLake{records: []map[string]any{{"id": "1"}, {"id": "2"}}}
	c := NewRPCClient(ft, lake, "x")

	result, err := c.Invoke(context.Background(), InvokeRequest{
		InstanceID:             "i",
		Operation:              "list_accounts",
		ResultStorageRequested: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("want 2 records, got %d", len(result.Records))
	}
	if lake.provider != "file" || lake.key != "k/123" {
		t.Fatalf("lake addressed wrong: provider=%q key=%q", lake.provider, lake.key)
	}
}

func TestInvoke_ResultStorageRef_NoLakeConfigured_Error(t *testing.T) {
	ft := &fakeTransport{reply: []byte(`{"status":"ok","result_storage_ref":{"provider":"file","storage_key":"k"}}`)}
	c := NewRPCClient(ft, nil, "x")
	_, err := c.Invoke(context.Background(), InvokeRequest{InstanceID: "i", Operation: "op"})
	if err == nil {
		t.Fatalf("expected error when reply uses storage_ref without LakeReader")
	}
}

func TestInvoke_RequiresInstanceAndOperation(t *testing.T) {
	c := NewRPCClient(&fakeTransport{}, nil, "x")
	if _, err := c.Invoke(context.Background(), InvokeRequest{Operation: "op"}); err == nil {
		t.Fatalf("expected error for missing instance_id")
	}
	if _, err := c.Invoke(context.Background(), InvokeRequest{InstanceID: "i"}); err == nil {
		t.Fatalf("expected error for missing operation")
	}
}

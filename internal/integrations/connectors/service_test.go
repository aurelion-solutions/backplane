// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type memRepo struct {
	rows         map[string]*ConnectorInstance
	deleteBefore time.Time
}

func newMemRepo() *memRepo { return &memRepo{rows: map[string]*ConnectorInstance{}} }

func (r *memRepo) GetByInstanceID(_ context.Context, id string) (*ConnectorInstance, error) {
	if row, ok := r.rows[id]; ok {
		return row, nil
	}
	return nil, ErrInstanceNotFound
}
func (r *memRepo) List(_ context.Context) ([]*ConnectorInstance, error) {
	out := make([]*ConnectorInstance, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	return out, nil
}
func (r *memRepo) ListOnline(_ context.Context, now time.Time) ([]*ConnectorInstance, error) {
	out := make([]*ConnectorInstance, 0, len(r.rows))
	for _, v := range r.rows {
		if v.IsOnlineAt(now) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *memRepo) Upsert(_ context.Context, id string, tags []string, descriptor map[string]any, now time.Time) (*ConnectorInstance, error) {
	existing, ok := r.rows[id]
	if !ok {
		existing = &ConnectorInstance{ID: uuid.New(), InstanceID: id, CreatedAt: now}
		r.rows[id] = existing
	}
	existing.Tags = tags
	existing.LastSeenAt = now
	existing.UpdatedAt = now
	if descriptor != nil {
		existing.Descriptor = descriptor
	}
	return existing, nil
}
func (r *memRepo) DeleteStale(_ context.Context, before time.Time) (int, error) {
	r.deleteBefore = before
	removed := 0
	for k, v := range r.rows {
		if v.LastSeenAt.Before(before) {
			delete(r.rows, k)
			removed++
		}
	}
	return removed, nil
}

func TestRegisterFromMessage_InsertsAndStampsTags(t *testing.T) {
	repo := newMemRepo()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo, func() time.Time { return now })

	inst, err := svc.RegisterFromMessage(context.Background(), RegistrationMessage{
		EventType:  EventTypeRegistered,
		InstanceID: "sf-prod-1",
		Tags:       []string{"salesforce", "prod"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.InstanceID != "sf-prod-1" || len(inst.Tags) != 2 {
		t.Fatalf("unexpected instance: %+v", inst)
	}
	if !inst.IsOnlineAt(now) {
		t.Fatalf("freshly registered instance must be online")
	}
}

func TestRegisterFromMessage_Heartbeat_KeepsDescriptor(t *testing.T) {
	repo := newMemRepo()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo, func() time.Time { return now })

	descriptor := &CapabilityDescriptor{
		Operations:          []OperationDescriptor{{Kind: "account_create"}},
		VerifyFactSupported: true,
	}
	if _, err := svc.RegisterFromMessage(context.Background(), RegistrationMessage{
		EventType:  EventTypeRegistered,
		InstanceID: "sf-prod-1",
		Tags:       []string{"salesforce"},
		Descriptor: descriptor,
	}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	// Heartbeat with no descriptor must preserve the stored one.
	inst, err := svc.RegisterFromMessage(context.Background(), RegistrationMessage{
		EventType:  EventTypeHeartbeat,
		InstanceID: "sf-prod-1",
		Tags:       []string{"salesforce", "prod"},
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if inst.Descriptor == nil {
		t.Fatalf("heartbeat must preserve descriptor")
	}
}

func TestSelectForTags_PicksOnlineMatch(t *testing.T) {
	repo := newMemRepo()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo, func() time.Time { return now })

	repo.rows["online-1"] = &ConnectorInstance{InstanceID: "online-1", Tags: []string{"salesforce", "prod"}, LastSeenAt: now.Add(-30 * time.Second)}
	repo.rows["online-2"] = &ConnectorInstance{InstanceID: "online-2", Tags: []string{"github"}, LastSeenAt: now.Add(-10 * time.Second)}
	repo.rows["offline"] = &ConnectorInstance{InstanceID: "offline", Tags: []string{"salesforce", "prod"}, LastSeenAt: now.Add(-1 * time.Hour)}

	pick, err := svc.SelectForTags(context.Background(), []string{"salesforce"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pick.InstanceID != "online-1" {
		t.Fatalf("expected online-1, got %q", pick.InstanceID)
	}
}

func TestSelectForTags_NoMatch_ReturnsError(t *testing.T) {
	repo := newMemRepo()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo, func() time.Time { return now })
	repo.rows["x"] = &ConnectorInstance{InstanceID: "x", Tags: []string{"github"}, LastSeenAt: now}

	_, err := svc.SelectForTags(context.Background(), []string{"salesforce"})
	if !errors.Is(err, ErrNoMatchingInstance) {
		t.Fatalf("want ErrNoMatchingInstance, got %v", err)
	}
}

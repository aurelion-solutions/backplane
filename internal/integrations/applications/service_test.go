// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package applications

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/google/uuid"
)

type memRepo struct {
	rows map[uuid.UUID]*Application
}

func newMemRepo() *memRepo { return &memRepo{rows: map[uuid.UUID]*Application{}} }

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Application, error) {
	if row, ok := r.rows[id]; ok {
		return row, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) GetByCode(_ context.Context, code string) (*Application, error) {
	for _, row := range r.rows {
		if row.Code == code {
			return row, nil
		}
	}
	return nil, ErrNotFound
}
func (r *memRepo) List(_ context.Context) ([]*Application, error) {
	out := make([]*Application, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	return out, nil
}
func (r *memRepo) Insert(_ context.Context, app *Application) error {
	for _, existing := range r.rows {
		if existing.Code == app.Code {
			return errors.New("uq_applications_code: simulated")
		}
	}
	r.rows[app.ID] = app
	return nil
}
func (r *memRepo) Update(_ context.Context, app *Application) error {
	r.rows[app.ID] = app
	return nil
}
func (r *memRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(r.rows, id)
	return nil
}

type recordingSink struct{ envelopes []events.Envelope }

func (s *recordingSink) Emit(_ context.Context, e events.Envelope) error {
	s.envelopes = append(s.envelopes, e)
	return nil
}

func TestCreate_Defaults(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo, nil, func() time.Time { return time.Unix(0, 0).UTC() })

	app, err := svc.Create(context.Background(), CreatePayload{Name: "Salesforce", Code: "salesforce"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !app.IsActive {
		t.Fatalf("default IsActive must be true")
	}
	if app.Config == nil || app.RequiredConnectorTags == nil {
		t.Fatalf("Config and RequiredConnectorTags must be non-nil zero values, got %+v", app)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc := NewService(newMemRepo(), nil, nil)
	name := "X"
	_, err := svc.Update(context.Background(), uuid.New(), PatchPayload{Name: &name})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdate_PatchAppliesOnlyProvidedFields(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo, nil, func() time.Time { return time.Unix(0, 0).UTC() })

	app, _ := svc.Create(context.Background(), CreatePayload{Name: "old", Code: "x"})
	newName := "renamed"
	updated, err := svc.Update(context.Background(), app.ID, PatchPayload{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if updated.Name != "renamed" {
		t.Fatalf("name not updated: %q", updated.Name)
	}
	if updated.Code != "x" {
		t.Fatalf("untouched code must remain: %q", updated.Code)
	}
}

func TestDecommission_EmitsEvent_WithCorrelationFromContext(t *testing.T) {
	repo := newMemRepo()
	sink := &recordingSink{}
	svc := NewService(repo, sink, func() time.Time { return time.Unix(0, 0).UTC() })

	app, _ := svc.Create(context.Background(), CreatePayload{Name: "x", Code: "x"})
	ctx := correlation.WithID(context.Background(), "test-cid")
	updated, err := svc.Decommission(ctx, app.ID)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if updated.IsActive {
		t.Fatalf("decommission must set IsActive=false")
	}
	if len(sink.envelopes) != 1 {
		t.Fatalf("expected one event, got %d", len(sink.envelopes))
	}
	env := sink.envelopes[0]
	if env.EventType != "inventory.application.decommissioned" {
		t.Fatalf("unexpected event_type %q", env.EventType)
	}
	if env.CorrelationID != "test-cid" {
		t.Fatalf("correlation_id from ctx must propagate, got %q", env.CorrelationID)
	}
	if env.Payload["application_id"] != app.ID.String() {
		t.Fatalf("payload must carry application_id")
	}
}

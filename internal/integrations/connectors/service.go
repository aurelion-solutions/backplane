// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"context"
	"time"
)

// StaleCutoff is how long an instance can be silent before its
// registration row is garbage-collected. Matches kernel's 1-day window.
const StaleCutoff = 24 * time.Hour

// Service holds the connector-registry use cases. The registration
// consumer and HTTP handlers call into this layer; the RPC client lives
// alongside the service (see rpc_client.go) but does NOT depend on it.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService wires the use case. now may be nil — the default is
// time.Now (UTC).
func NewService(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, now: now}
}

// RegisterFromMessage upserts a registration row and returns the merged
// instance. Also opportunistically deletes stale rows (same trigger as
// kernel's cleanup_stale_instances).
func (s *Service) RegisterFromMessage(ctx context.Context, msg RegistrationMessage) (*ConnectorInstance, error) {
	if err := msg.Validate(); err != nil {
		return nil, err
	}
	now := s.now()
	if _, err := s.repo.DeleteStale(ctx, now.Add(-StaleCutoff)); err != nil {
		return nil, err
	}
	var descriptor map[string]any
	if msg.Descriptor != nil {
		descriptor = descriptorToMap(*msg.Descriptor)
	}
	return s.repo.Upsert(ctx, msg.InstanceID, msg.Tags, descriptor, now)
}

// Get returns the instance by external instance_id or ErrInstanceNotFound.
func (s *Service) Get(ctx context.Context, instanceID string) (*ConnectorInstance, error) {
	return s.repo.GetByInstanceID(ctx, instanceID)
}

// List returns every registered instance ordered by instance_id ASC.
func (s *Service) List(ctx context.Context) ([]*ConnectorInstance, error) {
	return s.repo.List(ctx)
}

// ListOnline returns instances within the online threshold of now().
func (s *Service) ListOnline(ctx context.Context) ([]*ConnectorInstance, error) {
	return s.repo.ListOnline(ctx, s.now())
}

// SelectForTags picks the first online instance whose tag set covers
// requiredTags, or ErrNoMatchingInstance if none qualify. Also runs the
// stale cleanup pass so the live pool stays honest.
func (s *Service) SelectForTags(ctx context.Context, requiredTags []string) (*ConnectorInstance, error) {
	now := s.now()
	if _, err := s.repo.DeleteStale(ctx, now.Add(-StaleCutoff)); err != nil {
		return nil, err
	}
	online, err := s.repo.ListOnline(ctx, now)
	if err != nil {
		return nil, err
	}
	pick := Pick(online, requiredTags)
	if pick == nil {
		return nil, ErrNoMatchingInstance
	}
	return pick, nil
}

// CleanupStale removes every instance with last_seen_at older than
// StaleCutoff relative to now(). Returns the row count deleted.
func (s *Service) CleanupStale(ctx context.Context) (int, error) {
	return s.repo.DeleteStale(ctx, s.now().Add(-StaleCutoff))
}

// MatchingForTags returns every online instance covering requiredTags
// in the kernel wire shape. Implements the applications.MatchingProvider
// contract without forcing the applications package to import
// connectors directly.
func (s *Service) MatchingForTags(ctx context.Context, requiredTags []string, onlineOnly bool) ([]*ConnectorInstance, error) {
	now := s.now()
	var pool []*ConnectorInstance
	var err error
	if onlineOnly {
		pool, err = s.repo.ListOnline(ctx, now)
	} else {
		pool, err = s.repo.List(ctx)
	}
	if err != nil {
		return nil, err
	}
	return Matching(pool, requiredTags), nil
}

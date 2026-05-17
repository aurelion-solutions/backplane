// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/platform/storage"
	"github.com/google/uuid"
)

// Lake is the slice of platform/storage.Storage this engine actually
// needs. Defined as an interface so tests can stub the lake without
// dragging in DuckDB.
type Lake interface {
	AntiJoin(ctx context.Context, datasetType string, candidates []storage.Candidate) (storage.AntiJoinResult, error)
	WriteBatch(ctx context.Context, datasetType string, records []map[string]any) (string, error)
}

// EventSink mirrors core/events.Sink.
type EventSink interface {
	Emit(ctx context.Context, env events.Envelope) error
}

// Service is the use-case layer. The single public method is
// Process — every transport (HTTP handler, MQ consumer, discover)
// calls it.
type Service struct {
	repo  Repository
	lake  Lake
	sink  EventSink
	idGen func() uuid.UUID
	now   func() time.Time
}

// Deps bundles construction-time dependencies.
type Deps struct {
	Repo  Repository
	Lake  Lake
	Sink  EventSink
	IDGen func() uuid.UUID
	Now   func() time.Time
}

// NewService wires the Service.
func NewService(d Deps) *Service {
	if d.IDGen == nil {
		d.IDGen = uuid.New
	}
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		repo:  d.Repo,
		lake:  d.Lake,
		sink:  d.Sink,
		idGen: d.IDGen,
		now:   d.Now,
	}
}

// Process is the pure-function heart of ingest.
//
// Steps:
//  1. Validate envelope and that every record carries external_id.
//  2. Hash each record (canonical JSON sha256).
//  3. Ask the lake which external_ids are new or whose hash changed.
//  4. Build the lake-row shape (external_id + meta + payload) only
//     for the new+changed subset.
//  5. Write that subset as one lake batch — or skip the write if
//     nothing changed.
//  6. Insert the audit row and emit inventory.ingest.batch_received.
func (s *Service) Process(ctx context.Context, req Request) (*Result, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	source := strings.TrimSpace(req.Source)
	datasetType := strings.TrimSpace(req.DatasetType)

	// CorrelationID resolution: explicit > context > generate.
	cid := strings.TrimSpace(req.CorrelationID)
	if cid == "" {
		ctx, cid = correlation.Ensure(ctx)
	} else {
		ctx = correlation.WithID(ctx, cid)
	}

	// Step 1+2: validate per-record + hash.
	type hashed struct {
		record map[string]any
		hash   string
	}
	byExternalID := make(map[string]hashed, len(req.Records))
	candidates := make([]storage.Candidate, 0, len(req.Records))
	for i, rec := range req.Records {
		extRaw, ok := rec["external_id"]
		if !ok {
			return nil, fmt.Errorf("%w: records[%d]", ErrMissingExternalID, i)
		}
		extID := stringifyKey(extRaw)
		if extID == "" {
			return nil, fmt.Errorf("%w: records[%d] external_id is empty", ErrMissingExternalID, i)
		}
		hashHex, err := canonicalHashHex(rec)
		if err != nil {
			return nil, fmt.Errorf("inventory_ingest: hash records[%d]: %w", i, err)
		}
		// If the same external_id appears twice in the same window,
		// the LAST occurrence wins. Downstream lake holds revisions;
		// within one Process call only the latest payload reaches it.
		byExternalID[extID] = hashed{record: rec, hash: hashHex}
		candidates = append(candidates, storage.Candidate{ExternalID: extID, Hash: hashHex})
	}

	// Step 3: anti-join.
	verdict, err := s.lake.AntiJoin(ctx, datasetType, candidates)
	if err != nil {
		return nil, fmt.Errorf("inventory_ingest: anti-join: %w", err)
	}

	// Step 4: shape the lake rows for the write-set.
	toWrite := make([]map[string]any, 0, len(verdict.NewIDs)+len(verdict.ChangedIDs))
	committedAt := s.now().UTC().Format(time.RFC3339Nano)
	for _, id := range verdict.NewIDs {
		h := byExternalID[id]
		toWrite = append(toWrite, lakeRow(id, h.hash, h.record, committedAt, cid))
	}
	for _, id := range verdict.ChangedIDs {
		h := byExternalID[id]
		toWrite = append(toWrite, lakeRow(id, h.hash, h.record, committedAt, cid))
	}

	// Step 5: write lake (or skip).
	var lakeRef *string
	if len(toWrite) > 0 {
		key, err := s.lake.WriteBatch(ctx, datasetType, toWrite)
		if err != nil {
			return nil, fmt.Errorf("inventory_ingest: write lake: %w", err)
		}
		lakeRef = &key
	}

	// Step 6: audit + event.
	batch := &IngestBatch{
		ID:            s.idGen(),
		Source:        source,
		DatasetType:   datasetType,
		CorrelationID: cid,
		ReceivedCount: len(req.Records),
		WrittenCount:  len(toWrite),
		SkippedCount:  len(req.Records) - len(toWrite),
		NewCount:      len(verdict.NewIDs),
		ChangedCount:  len(verdict.ChangedIDs),
		LakeRef:       lakeRef,
		CompletedAt:   s.now(),
	}
	if err := s.repo.Insert(ctx, batch); err != nil {
		return nil, fmt.Errorf("inventory_ingest: insert batch audit: %w", err)
	}
	if !req.SkipEvent {
		if err := s.emitReceived(ctx, batch); err != nil {
			return nil, err
		}
	}

	return &Result{
		BatchID:       batch.ID.String(),
		Source:        batch.Source,
		DatasetType:   batch.DatasetType,
		CorrelationID: batch.CorrelationID,
		Received:      batch.ReceivedCount,
		Written:       batch.WrittenCount,
		Skipped:       batch.SkippedCount,
		New:           batch.NewCount,
		Changed:       batch.ChangedCount,
		LakeRef:       batch.LakeRef,
	}, nil
}

// Get returns one batch audit row.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*IngestBatch, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns paginated audit rows + total count.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*IngestBatch, int, error) {
	return s.repo.List(ctx, limit, offset)
}

// lakeRow wraps one source record into the canonical lake shape:
// top-level external_id (for DuckDB filtering), meta (backplane-added),
// payload (verbatim from caller, including external_id).
func lakeRow(externalID, hash string, payload map[string]any, committedAt, correlationID string) map[string]any {
	return map[string]any{
		"external_id": externalID,
		"meta": map[string]any{
			"hash":           hash,
			"committed_at":   committedAt,
			"correlation_id": correlationID,
		},
		"payload": payload,
	}
}

// stringifyKey coerces a JSON-decoded external_id value to its string
// form. Accepts strings outright, numbers as their plain decimal,
// booleans literally; anything else returns empty so the caller errors.
func stringifyKey(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		// JSON-decoded numbers come through as float64. Keep both
		// integer and decimal cases stable: "%v" gives "123" not
		// "1.23e+02" for small ints.
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func (s *Service) emitReceived(ctx context.Context, b *IngestBatch) error {
	if s.sink == nil {
		return nil
	}
	payload := map[string]any{
		"batch_id":     b.ID.String(),
		"source":       b.Source,
		"dataset_type": b.DatasetType,
		"received":     b.ReceivedCount,
		"written":      b.WrittenCount,
		"skipped":      b.SkippedCount,
		"new":          b.NewCount,
		"changed":      b.ChangedCount,
	}
	if b.LakeRef != nil {
		payload["lake_ref"] = *b.LakeRef
	}
	env, err := events.NewEnvelope(events.EnvelopeInput{
		EventType:     EventBatchReceived,
		CorrelationID: b.CorrelationID,
		Payload:       payload,
		ActorKind:     events.ParticipantComponent,
		ActorID:       EventActorComponent,
		TargetKind:    events.ParticipantCapability,
		TargetID:      b.ID.String(),
	})
	if err != nil {
		return fmt.Errorf("inventory_ingest: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("inventory_ingest: emit event: %w", err)
	}
	return nil
}

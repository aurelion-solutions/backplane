// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_records

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeRepo is a minimal in-memory implementation of Repository used by
// resolver tests only — it ignores BulkUpsert beyond what these tests
// need.
type fakeRepo struct {
	records  map[uuid.UUID]*EmployeeRecord
	attrs    map[uuid.UUID]map[string]string
	mappings []*EmployeeProviderAttributeMapping
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		records: map[uuid.UUID]*EmployeeRecord{},
		attrs:   map[uuid.UUID]map[string]string{},
	}
}

func (r *fakeRepo) GetRecordByID(_ context.Context, id uuid.UUID) (*EmployeeRecord, error) {
	if rec, ok := r.records[id]; ok {
		return rec, nil
	}
	return nil, ErrNotFound
}
func (r *fakeRepo) GetRecordByExternal(_ context.Context, _ uuid.UUID, _ string) (*EmployeeRecord, error) {
	return nil, ErrNotFound
}
func (r *fakeRepo) ListRecords(_ context.Context) ([]*EmployeeRecord, error) {
	out := []*EmployeeRecord{}
	for _, v := range r.records {
		out = append(out, v)
	}
	return out, nil
}
func (r *fakeRepo) InsertRecord(_ context.Context, row *EmployeeRecord) error {
	r.records[row.ID] = row
	return nil
}
func (r *fakeRepo) ListRecordAttributes(_ context.Context, id uuid.UUID) ([]*EmployeeRecordAttribute, error) {
	out := []*EmployeeRecordAttribute{}
	for k, v := range r.attrs[id] {
		out = append(out, &EmployeeRecordAttribute{
			ID: uuid.New(), EmployeeRecordID: id, Key: k, Value: v,
		})
	}
	return out, nil
}
func (r *fakeRepo) GetRecordAttribute(_ context.Context, id uuid.UUID, key string) (*EmployeeRecordAttribute, error) {
	if v, ok := r.attrs[id][key]; ok {
		return &EmployeeRecordAttribute{ID: uuid.New(), EmployeeRecordID: id, Key: key, Value: v}, nil
	}
	return nil, ErrAttributeNotFound
}
func (r *fakeRepo) UpsertRecordAttribute(_ context.Context, a *EmployeeRecordAttribute) error {
	if r.attrs[a.EmployeeRecordID] == nil {
		r.attrs[a.EmployeeRecordID] = map[string]string{}
	}
	r.attrs[a.EmployeeRecordID][a.Key] = a.Value
	return nil
}
func (r *fakeRepo) DeleteRecordAttribute(_ context.Context, id uuid.UUID, key string) error {
	delete(r.attrs[id], key)
	return nil
}
func (r *fakeRepo) GetMappingByID(_ context.Context, id uuid.UUID) (*EmployeeProviderAttributeMapping, error) {
	for _, m := range r.mappings {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, ErrMappingNotFound
}
func (r *fakeRepo) ListMappings(_ context.Context, appID uuid.UUID, isDet, allowUp *bool) ([]*EmployeeProviderAttributeMapping, error) {
	out := []*EmployeeProviderAttributeMapping{}
	for _, m := range r.mappings {
		if m.ApplicationID != appID {
			continue
		}
		if isDet != nil && m.IsDeterminator != *isDet {
			continue
		}
		if allowUp != nil && m.AllowUpstream != *allowUp {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}
func (r *fakeRepo) InsertMapping(_ context.Context, m *EmployeeProviderAttributeMapping) error {
	r.mappings = append(r.mappings, m)
	return nil
}
func (r *fakeRepo) DeleteMapping(_ context.Context, id uuid.UUID) error {
	out := r.mappings[:0]
	for _, m := range r.mappings {
		if m.ID != id {
			out = append(out, m)
		}
	}
	r.mappings = out
	return nil
}
func (r *fakeRepo) GetMatchByRecord(_ context.Context, _ uuid.UUID) (*EmployeeRecordMatch, error) {
	return nil, ErrNotFound
}
func (r *fakeRepo) ListMatches(_ context.Context) ([]*EmployeeRecordMatch, error) {
	return nil, nil
}
func (r *fakeRepo) UpsertMatch(_ context.Context, _ *EmployeeRecordMatch) error {
	return nil
}
func (r *fakeRepo) DeleteMatch(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (r *fakeRepo) FindUpstreamPeers(_ context.Context, recordID, excludeID uuid.UUID) ([]uuid.UUID, error) {
	rec, ok := r.records[recordID]
	if !ok {
		return nil, nil
	}
	keys := map[string]struct{}{}
	for _, m := range r.mappings {
		if m.ApplicationID == rec.ApplicationID && m.AllowUpstream {
			keys[m.EmployeeRecordKey] = struct{}{}
		}
	}
	selfAttrs := r.attrs[recordID]
	if len(selfAttrs) == 0 || len(keys) == 0 {
		return nil, nil
	}
	peers := map[uuid.UUID]struct{}{}
	for otherID, attrs := range r.attrs {
		if otherID == excludeID {
			continue
		}
		for k, v := range attrs {
			if _, ok := keys[k]; !ok {
				continue
			}
			if selfV, ok := selfAttrs[k]; ok && selfV == v {
				peers[otherID] = struct{}{}
				break
			}
		}
	}
	out := make([]uuid.UUID, 0, len(peers))
	for id := range peers {
		out = append(out, id)
	}
	return out, nil
}
func (r *fakeRepo) BulkUpsert(_ context.Context, _ []BulkItem, _ ApplicationResolver, _ func() uuid.UUID) (int, error) {
	return 0, nil
}

// fakePersonAPI lets resolver tests assert FindPersonByAttribute /
// CreatePersonWithEmployment / PropagateAttribute / PrimaryEmploymentForPerson.
type fakePersonAPI struct {
	// keyValueIndex maps (key|value) -> personID
	keyValueIndex map[string]uuid.UUID
	// employmentForPerson maps personID -> employmentID
	employmentForPerson map[uuid.UUID]uuid.UUID
	created             []personCreated
	propagated          []propagation
}

type personCreated struct {
	key, value   string
	personID     uuid.UUID
	employmentID uuid.UUID
}
type propagation struct {
	personID   uuid.UUID
	key, value string
}

func newFakePersons() *fakePersonAPI {
	return &fakePersonAPI{
		keyValueIndex:       map[string]uuid.UUID{},
		employmentForPerson: map[uuid.UUID]uuid.UUID{},
	}
}

func (f *fakePersonAPI) FindPersonByAttribute(_ context.Context, key, value string) (uuid.UUID, bool, error) {
	id, ok := f.keyValueIndex[key+"|"+value]
	return id, ok, nil
}
func (f *fakePersonAPI) CreatePersonWithEmployment(_ context.Context, key, value string) (uuid.UUID, uuid.UUID, error) {
	pid := uuid.New()
	eid := uuid.New()
	f.keyValueIndex[key+"|"+value] = pid
	f.employmentForPerson[pid] = eid
	f.created = append(f.created, personCreated{key, value, pid, eid})
	return pid, eid, nil
}
func (f *fakePersonAPI) PropagateAttribute(_ context.Context, personID uuid.UUID, key, value string) error {
	f.propagated = append(f.propagated, propagation{personID, key, value})
	return nil
}
func (f *fakePersonAPI) PrimaryEmploymentForPerson(_ context.Context, personID uuid.UUID) (uuid.UUID, bool, error) {
	eid, ok := f.employmentForPerson[personID]
	return eid, ok, nil
}

func newResolverFixture() (*Resolver, *fakeRepo, *fakePersonAPI) {
	repo := newFakeRepo()
	persons := newFakePersons()
	return NewResolver(repo, persons), repo, persons
}

func TestResolver_DirectDeterminator_MatchesExistingPerson(t *testing.T) {
	res, repo, persons := newResolverFixture()
	appID := uuid.New()
	recordID := uuid.New()
	personID := uuid.New()
	employmentID := uuid.New()

	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID, ExternalID: "ext-1"}
	repo.attrs[recordID] = map[string]string{"email_src": "alice@x.com"}
	repo.mappings = []*EmployeeProviderAttributeMapping{
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "email_src", PersonKey: "email", IsDeterminator: true},
	}
	persons.keyValueIndex["email|alice@x.com"] = personID
	persons.employmentForPerson[personID] = employmentID

	out, err := res.Resolve(context.Background(), recordID)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.PersonID != personID || out.EmploymentID != employmentID || !out.ViaDeterminator {
		t.Fatalf("expected via-determinator match (person=%v, emp=%v), got %+v", personID, employmentID, out)
	}
	if len(persons.created) != 0 {
		t.Fatalf("expected no creation, got %+v", persons.created)
	}
}

func TestResolver_DirectDeterminator_CreatesNewPersonAndEmployment(t *testing.T) {
	res, repo, persons := newResolverFixture()
	appID := uuid.New()
	recordID := uuid.New()

	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID, ExternalID: "ext-1"}
	repo.attrs[recordID] = map[string]string{"email_src": "new@x.com"}
	repo.mappings = []*EmployeeProviderAttributeMapping{
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "email_src", PersonKey: "email", IsDeterminator: true},
	}

	out, err := res.Resolve(context.Background(), recordID)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.PersonID == uuid.Nil || out.EmploymentID == uuid.Nil || !out.ViaDeterminator {
		t.Fatalf("expected determinator-created match, got %+v", out)
	}
	if len(persons.created) != 1 || persons.created[0].key != "email" || persons.created[0].value != "new@x.com" {
		t.Fatalf("expected one create, got %+v", persons.created)
	}
}

func TestResolver_UpstreamPeer_Resolves(t *testing.T) {
	res, repo, persons := newResolverFixture()
	appID := uuid.New()
	selfID := uuid.New()
	peerID := uuid.New()
	personID := uuid.New()
	employmentID := uuid.New()

	repo.records[selfID] = &EmployeeRecord{ID: selfID, ApplicationID: appID, ExternalID: "ext-self"}
	repo.records[peerID] = &EmployeeRecord{ID: peerID, ApplicationID: appID, ExternalID: "ext-peer"}
	repo.attrs[selfID] = map[string]string{"shared": "S-001"}
	repo.attrs[peerID] = map[string]string{"shared": "S-001", "email_src": "x@x.com"}
	repo.mappings = []*EmployeeProviderAttributeMapping{
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "shared", PersonKey: "_unused_", AllowUpstream: true},
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "email_src", PersonKey: "email", IsDeterminator: true},
	}
	persons.keyValueIndex["email|x@x.com"] = personID
	persons.employmentForPerson[personID] = employmentID

	out, err := res.Resolve(context.Background(), selfID)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.PersonID != personID || out.EmploymentID != employmentID || out.ViaDeterminator {
		t.Fatalf("expected via-upstream match (person=%v, emp=%v, via_det=false), got %+v", personID, employmentID, out)
	}
}

func TestResolver_UnresolvedReturnsNil(t *testing.T) {
	res, repo, _ := newResolverFixture()
	appID := uuid.New()
	id := uuid.New()
	repo.records[id] = &EmployeeRecord{ID: id, ApplicationID: appID, ExternalID: "ext"}
	repo.attrs[id] = map[string]string{"weird": "thing"}
	out, err := res.Resolve(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("expected nil outcome, got %+v", out)
	}
}

func TestResolver_NonDeterminatorAttributes_PropagateToPerson(t *testing.T) {
	res, repo, persons := newResolverFixture()
	appID := uuid.New()
	recordID := uuid.New()
	personID := uuid.New()
	employmentID := uuid.New()

	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID, ExternalID: "ext"}
	repo.attrs[recordID] = map[string]string{
		"email_src":     "alice@x.com",
		"job_title_src": "Engineer",
	}
	repo.mappings = []*EmployeeProviderAttributeMapping{
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "email_src", PersonKey: "email", IsDeterminator: true},
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "job_title_src", PersonKey: "job_title"},
	}
	persons.keyValueIndex["email|alice@x.com"] = personID
	persons.employmentForPerson[personID] = employmentID

	out, err := res.Resolve(context.Background(), recordID)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.PersonID != personID {
		t.Fatalf("expected match, got %+v", out)
	}
	if len(persons.propagated) != 1 || persons.propagated[0].key != "job_title" || persons.propagated[0].value != "Engineer" {
		t.Fatalf("expected one propagation of job_title=Engineer, got %+v", persons.propagated)
	}
}

func TestResolver_CycleStopsAtVisited(t *testing.T) {
	res, repo, _ := newResolverFixture()
	appID := uuid.New()
	a := uuid.New()
	b := uuid.New()

	repo.records[a] = &EmployeeRecord{ID: a, ApplicationID: appID, ExternalID: "a"}
	repo.records[b] = &EmployeeRecord{ID: b, ApplicationID: appID, ExternalID: "b"}
	repo.attrs[a] = map[string]string{"shared": "X"}
	repo.attrs[b] = map[string]string{"shared": "X"}
	repo.mappings = []*EmployeeProviderAttributeMapping{
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "shared", PersonKey: "_unused_", AllowUpstream: true},
	}

	out, err := res.Resolve(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("expected nil after cycle, got %+v", out)
	}
}

func TestResolver_RepeatedProcessing_Stable(t *testing.T) {
	res, repo, persons := newResolverFixture()
	appID := uuid.New()
	recordID := uuid.New()
	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID, ExternalID: "ext"}
	repo.attrs[recordID] = map[string]string{"email_src": "stable@x.com"}
	repo.mappings = []*EmployeeProviderAttributeMapping{
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "email_src", PersonKey: "email", IsDeterminator: true},
	}

	first, err := res.Resolve(context.Background(), recordID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := res.Resolve(context.Background(), recordID)
	if err != nil {
		t.Fatal(err)
	}
	if first.PersonID != second.PersonID || first.EmploymentID != second.EmploymentID {
		t.Fatalf("expected stable ids, got %v/%v vs %v/%v",
			first.PersonID, first.EmploymentID, second.PersonID, second.EmploymentID)
	}
	if len(persons.created) != 1 {
		t.Fatalf("expected exactly one creation across two runs, got %d", len(persons.created))
	}
}

func TestResolver_MissingRecord_ReturnsNil(t *testing.T) {
	res, _, _ := newResolverFixture()
	out, err := res.Resolve(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("expected nil for unknown record, got %+v", out)
	}
}

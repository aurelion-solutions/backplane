// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_profile

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRepo is an in-memory Repository for deterministic service tests.
type fakeRepo struct {
	person      *personRow
	employments []employmentRow
	principals  []principalRow
	accounts    []accountRow
	grants      []grantRow
	initiatives []initiativeRow
	personErr   error
}

func (f *fakeRepo) Person(_ context.Context, _ uuid.UUID) (*personRow, error) {
	if f.personErr != nil {
		return nil, f.personErr
	}
	return f.person, nil
}
func (f *fakeRepo) Employments(_ context.Context, _ uuid.UUID) ([]employmentRow, error) {
	return f.employments, nil
}
func (f *fakeRepo) EmploymentPrincipals(_ context.Context, _ []uuid.UUID) ([]principalRow, error) {
	return f.principals, nil
}
func (f *fakeRepo) Accounts(_ context.Context, _ []uuid.UUID) ([]accountRow, error) {
	return f.accounts, nil
}
func (f *fakeRepo) Grants(_ context.Context, _ []uuid.UUID) ([]grantRow, error) {
	return f.grants, nil
}
func (f *fakeRepo) Initiatives(_ context.Context, _ []uuid.UUID) ([]initiativeRow, error) {
	return f.initiatives, nil
}

func strptr(s string) *string     { return &s }
func tptr(t time.Time) *time.Time { return &t }

// now is pinned so "expired"/"active" are deterministic.
var fixedNow = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func richFixture() *fakeRepo {
	p1 := uuid.New()
	e1 := uuid.New()
	pr1 := uuid.New()
	a1 := uuid.New()
	a2 := uuid.New()
	appAWS := uuid.New()
	appOkta := uuid.New()
	return &fakeRepo{
		person:      &personRow{ID: p1, ExternalID: "PERS-1", FullName: "Marcus Vale"},
		employments: []employmentRow{{ID: e1, Code: "primary", StartDate: fixedNow.AddDate(-2, 0, 0), EndDate: nil}},
		principals:  []principalRow{{ID: pr1, EmploymentID: e1}},
		accounts: []accountRow{
			{ID: a1, PrincipalID: pr1, ApplicationID: appAWS, ApplicationName: "AWS", ApplicationCode: "aws", Username: "mvale", IsActive: true, IsPrivileged: true, MFAEnabled: true, EffectiveState: "active"},
			{ID: a2, PrincipalID: pr1, ApplicationID: appOkta, ApplicationName: "Okta", ApplicationCode: "okta", Username: "m.vale", IsActive: true, IsPrivileged: false, MFAEnabled: false, EffectiveState: "active"},
		},
		grants: []grantRow{
			{AccountID: a1, CapabilitySlug: "read", CapabilityName: "Read", ScopeKeyCode: "global", ScopeValue: nil},
			{AccountID: a1, CapabilitySlug: "admin", CapabilityName: "Admin", ScopeKeyCode: "region", ScopeValue: strptr("eu")},
		},
		initiatives: []initiativeRow{
			{ID: uuid.New(), PrincipalID: pr1, ApplicationID: appAWS, ApplicationName: "AWS", ApplicationCode: "aws", CapabilityName: strptr("Admin"), Kind: "requested", Actor: "alice", ValidFrom: fixedNow.AddDate(-1, 0, 0), ValidUntil: tptr(fixedNow.AddDate(0, -1, 0))},
			{ID: uuid.New(), PrincipalID: pr1, ApplicationID: appOkta, ApplicationName: "Okta", ApplicationCode: "okta", CapabilityName: nil, Kind: "inheritance", Actor: "system", ValidFrom: fixedNow.AddDate(-1, 0, 0), ValidUntil: tptr(fixedNow.AddDate(0, 6, 0))},
		},
	}
}

func TestLoadAssemblesNestedProfile(t *testing.T) {
	svc := NewService(richFixture())
	svc.now = func() time.Time { return fixedNow }

	got, err := svc.Load(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.FullName != "Marcus Vale" {
		t.Fatalf("FullName = %q", got.FullName)
	}
	if got.Terminated {
		t.Fatalf("person with an open employment must not be Terminated")
	}
	if len(got.Applications) != 2 {
		t.Fatalf("apps = %d, want 2", len(got.Applications))
	}
	// sorted by name: AWS, Okta
	if got.Applications[0].ApplicationName != "AWS" || got.Applications[1].ApplicationName != "Okta" {
		t.Fatalf("apps not sorted by name: %+v", []string{got.Applications[0].ApplicationName, got.Applications[1].ApplicationName})
	}
	aws := got.Applications[0]
	if len(aws.Accounts) != 1 || len(aws.Accounts[0].Grants) != 2 {
		t.Fatalf("AWS account/grant shape wrong: %+v", aws)
	}
	if len(aws.Initiatives) != 1 || !aws.Initiatives[0].Expired {
		t.Fatalf("AWS initiative should be expired: %+v", aws.Initiatives)
	}
	okta := got.Applications[1]
	if okta.Initiatives[0].CapabilityName != nil {
		t.Fatalf("Okta initiative should be account-level (nil capability)")
	}
	if okta.Initiatives[0].Expired {
		t.Fatalf("Okta initiative valid 6 months out must not be expired")
	}
}

func TestLoadTerminatedWhenNoActiveEmployment(t *testing.T) {
	f := richFixture()
	end := fixedNow.AddDate(0, -2, 0)
	f.employments[0].EndDate = &end
	f.accounts = nil
	f.grants = nil
	f.initiatives = nil
	svc := NewService(f)
	svc.now = func() time.Time { return fixedNow }

	got, err := svc.Load(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Terminated {
		t.Fatalf("person whose only employment ended must be Terminated")
	}
	if got.Employments[0].Active {
		t.Fatalf("ended employment must not be Active")
	}
}

func TestLoadPropagatesPersonNotFound(t *testing.T) {
	svc := NewService(&fakeRepo{personErr: ErrPersonNotFound})
	if _, err := svc.Load(context.Background(), uuid.New()); err != ErrPersonNotFound {
		t.Fatalf("err = %v, want ErrPersonNotFound", err)
	}
}

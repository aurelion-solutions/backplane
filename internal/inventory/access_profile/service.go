// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_profile

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Service assembles the nested access profile from the flat row sets
// the repository returns. It is read-only and emits no events.
type Service struct {
	repo Repository
	// now is injectable so tests can pin "today"; defaults to time.Now.
	now func() time.Time
}

// NewService constructs a Service over the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Load builds the full access profile for one person. Returns
// ErrPersonNotFound when the person id does not resolve.
func (s *Service) Load(ctx context.Context, personID uuid.UUID) (*AccessProfile, error) {
	person, err := s.repo.Person(ctx, personID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()

	emps, err := s.repo.Employments(ctx, personID)
	if err != nil {
		return nil, err
	}
	profile := &AccessProfile{
		PersonID:     person.ID,
		ExternalID:   person.ExternalID,
		FullName:     person.FullName,
		Employments:  make([]EmploymentView, 0, len(emps)),
		Applications: []ApplicationView{},
	}
	empIDs := make([]uuid.UUID, 0, len(emps))
	anyActive := false
	for _, e := range emps {
		active := e.EndDate == nil || e.EndDate.After(now)
		anyActive = anyActive || active
		profile.Employments = append(profile.Employments, EmploymentView{
			ID:        e.ID,
			Code:      e.Code,
			StartDate: e.StartDate,
			EndDate:   e.EndDate,
			Active:    active,
		})
		empIDs = append(empIDs, e.ID)
	}
	profile.Terminated = !anyActive && len(emps) > 0

	principals, err := s.repo.EmploymentPrincipals(ctx, empIDs)
	if err != nil {
		return nil, err
	}
	principalIDs := make([]uuid.UUID, 0, len(principals))
	for _, p := range principals {
		principalIDs = append(principalIDs, p.ID)
	}

	accounts, err := s.repo.Accounts(ctx, principalIDs)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]uuid.UUID, 0, len(accounts))
	for _, a := range accounts {
		accountIDs = append(accountIDs, a.ID)
	}

	grants, err := s.repo.Grants(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	grantsByAccount := make(map[uuid.UUID][]GrantView, len(accounts))
	for _, g := range grants {
		grantsByAccount[g.AccountID] = append(grantsByAccount[g.AccountID], GrantView{
			CapabilitySlug: g.CapabilitySlug,
			CapabilityName: g.CapabilityName,
			ScopeKeyCode:   g.ScopeKeyCode,
			ScopeValue:     g.ScopeValue,
		})
	}

	initiatives, err := s.repo.Initiatives(ctx, principalIDs)
	if err != nil {
		return nil, err
	}

	// Fold accounts + initiatives into per-application buckets.
	apps := map[uuid.UUID]*ApplicationView{}
	appOf := func(id uuid.UUID, name, code string) *ApplicationView {
		av := apps[id]
		if av == nil {
			av = &ApplicationView{
				ApplicationID:   id,
				ApplicationName: name,
				ApplicationCode: code,
				Accounts:        []AccountView{},
				Initiatives:     []InitiativeView{},
			}
			apps[id] = av
		}
		return av
	}

	for _, a := range accounts {
		av := appOf(a.ApplicationID, a.ApplicationName, a.ApplicationCode)
		gv := grantsByAccount[a.ID]
		if gv == nil {
			gv = []GrantView{}
		}
		av.Accounts = append(av.Accounts, AccountView{
			ID:             a.ID,
			Username:       a.Username,
			DisplayName:    a.DisplayName,
			IsActive:       a.IsActive,
			IsPrivileged:   a.IsPrivileged,
			MFAEnabled:     a.MFAEnabled,
			EffectiveState: a.EffectiveState,
			Grants:         gv,
		})
	}
	for _, in := range initiatives {
		av := appOf(in.ApplicationID, in.ApplicationName, in.ApplicationCode)
		av.Initiatives = append(av.Initiatives, InitiativeView{
			ID:             in.ID,
			Kind:           in.Kind,
			CapabilityName: in.CapabilityName,
			Actor:          in.Actor,
			ValidFrom:      in.ValidFrom,
			ValidUntil:     in.ValidUntil,
			Expired:        in.ValidUntil != nil && in.ValidUntil.Before(now),
		})
	}

	profile.Applications = make([]ApplicationView, 0, len(apps))
	for _, av := range apps {
		profile.Applications = append(profile.Applications, *av)
	}
	sort.Slice(profile.Applications, func(i, j int) bool {
		return profile.Applications[i].ApplicationName < profile.Applications[j].ApplicationName
	})

	return profile, nil
}

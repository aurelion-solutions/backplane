// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/inventory/employee_provider_mappings"
	"github.com/aurelion-solutions/backplane/internal/inventory/employment_record_matches"
	"github.com/aurelion-solutions/backplane/internal/inventory/employments"
	"github.com/aurelion-solutions/backplane/internal/inventory/org_units"
	"github.com/aurelion-solutions/backplane/internal/inventory/persons"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// LakeReader narrows storage.Storage to the single method the action
// needs.
type LakeReader interface {
	ReadBatch(ctx context.Context, storageKey string) ([]map[string]any, error)
}

// Deps is the composition-root injection set.
type Deps struct {
	Lake     LakeReader
	Mappings employee_provider_mappings.Repository
	Persons  persons.AttributeLookup
	OrgUnits org_units.Lookup
	Matches  employment_record_matches.Repository
}

// New builds the handler closure with deps bound.
func New(deps Deps) registry.Handler[Args, Result] {
	return func(args Args, ctx registry.ActionContext) (Result, error) {
		if args.LakeRef == "" {
			return Result{}, fmt.Errorf("employee: empty lake_ref")
		}
		if args.Source == "" {
			return Result{}, fmt.Errorf("employee: empty source")
		}
		records, err := deps.Lake.ReadBatch(ctx.Ctx, args.LakeRef)
		if err != nil {
			return Result{}, fmt.Errorf("employee: read lake %q: %w", args.LakeRef, err)
		}
		mappings, err := deps.Mappings.ListActiveByProvider(ctx.Ctx, ctx.Tx, args.Source)
		if err != nil {
			return Result{}, fmt.Errorf("employee: list mappings: %w", err)
		}
		if len(mappings) == 0 {
			return Result{Read: len(records), Skipped: len(records)}, nil
		}

		now := time.Now().UTC()
		r := &resolver{
			ctx:      ctx.Ctx,
			tx:       ctx.Tx,
			mappings: mappings,
			lookup:   deps.Persons,
			now:      now,
		}

		res := Result{Read: len(records)}
		for _, rec := range records {
			extID, payload, ok := parseRecord(rec)
			if !ok {
				res.Skipped++
				continue
			}

			outcome, found, err := r.resolve(payload)
			if err != nil {
				return Result{}, fmt.Errorf("employee: resolve: %w", err)
			}
			if !found {
				res.Unresolved++
				continue
			}
			if outcome.PersonWasCreated {
				res.PersonsCreated++
			}
			res.PersonsMatched++

			if err := propagateAttributes(ctx.Ctx, ctx.Tx, outcome.PersonID, mappings, payload); err != nil {
				return Result{}, fmt.Errorf("employee: propagate attrs: %w", err)
			}

			periods, _ := payload["employments"].([]any)
			for _, p := range periods {
				period, ok := p.(map[string]any)
				if !ok {
					res.Skipped++
					continue
				}
				parsed, ok := parsePeriod(period)
				if !ok {
					res.Skipped++
					continue
				}

				existing, err := deps.Matches.GetByKey(ctx.Ctx, ctx.Tx, args.Source, extID, parsed.StartDate)
				if err != nil && !errors.Is(err, employment_record_matches.ErrNotFound) {
					return Result{}, fmt.Errorf("employee: match lookup: %w", err)
				}
				if existing != nil {
					res.EmploymentsAlreadyMatched++
					continue
				}

				ouID, ouOK, err := resolveOrgUnit(ctx.Ctx, ctx.Tx, deps.OrgUnits, parsed.OrgUnitID, parsed.OrgUnitName)
				if err != nil {
					return Result{}, fmt.Errorf("employee: resolve org_unit: %w", err)
				}
				if !ouOK && (parsed.OrgUnitID != "" || parsed.OrgUnitName != "") {
					res.OrgUnitUnresolved++
				}

				code := "active"
				if parsed.EndDate != nil {
					code = "former"
				}

				empID, err := upsertEmployment(ctx.Ctx, ctx.Tx,
					outcome.PersonID, code,
					parsed.StartDate, parsed.EndDate,
					ouID, parsed.TitleName, now)
				if err != nil {
					return Result{}, fmt.Errorf("employee: upsert employment: %w", err)
				}

				if err := upsertEmploymentSidecars(ctx.Ctx, ctx.Tx, empID, parsed, ouOK); err != nil {
					return Result{}, fmt.Errorf("employee: upsert employment attrs: %w", err)
				}

				match := &employment_record_matches.EmploymentRecordMatch{
					ID:                     uuid.New(),
					EmploymentID:           empID,
					Source:                 args.Source,
					SourceRecordExternalID: extID,
					PeriodStartDate:        parsed.StartDate,
					MatchedViaDeterminator: outcome.MatchedViaDeterminator,
					CreatedAt:              now,
					UpdatedAt:              now,
				}
				if err := deps.Matches.Insert(ctx.Ctx, ctx.Tx, match); err != nil {
					return Result{}, fmt.Errorf("employee: insert match: %w", err)
				}
				res.EmploymentsAdded++
			}
		}
		return res, nil
	}
}

// parseRecord pulls (external_id, payload) from one lake record.
func parseRecord(rec map[string]any) (string, map[string]any, bool) {
	extID, _ := rec["external_id"].(string)
	payload, _ := rec["payload"].(map[string]any)
	if extID == "" || payload == nil {
		return "", nil, false
	}
	return extID, payload, true
}

// propagateAttributes upserts every non-determinator, mapped payload
// value into person_attributes.
func propagateAttributes(
	ctx context.Context, tx bun.IDB,
	personID uuid.UUID,
	mappings []*employee_provider_mappings.Mapping,
	payload map[string]any,
) error {
	for _, m := range mappings {
		if m.IsDeterminator {
			continue
		}
		value, _ := payload[m.RecordKey].(string)
		if value == "" {
			continue
		}
		if err := upsertPersonAttribute(ctx, tx, personID, m.PersonKey, value); err != nil {
			return err
		}
	}
	return nil
}

// parsedPeriod is the action's view of one employments[] element.
type parsedPeriod struct {
	StartDate   time.Time
	EndDate     *time.Time
	OrgUnitID   string
	OrgUnitName string
	TitleID     string
	TitleName   string
}

// parsePeriod accepts ISO date strings (YYYY-MM-DD). RFC3339 with
// time is also tolerated. Missing / unparseable start_date → false.
func parsePeriod(m map[string]any) (parsedPeriod, bool) {
	startStr, _ := m["start_date"].(string)
	if startStr == "" {
		return parsedPeriod{}, false
	}
	start, err := parseDate(startStr)
	if err != nil {
		return parsedPeriod{}, false
	}
	p := parsedPeriod{StartDate: start}
	if endStr, ok := m["end_date"].(string); ok && endStr != "" {
		if end, err := parseDate(endStr); err == nil {
			p.EndDate = &end
		}
	}
	p.OrgUnitID, _ = m["org_unit_id"].(string)
	p.OrgUnitName, _ = m["org_unit_name"].(string)
	p.TitleID, _ = m["title_id"].(string)
	p.TitleName, _ = m["title_name"].(string)
	return p, true
}

func parseDate(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Truncate(24 * time.Hour), nil
	}
	return time.Time{}, fmt.Errorf("employee: cannot parse date %q", s)
}

// resolveOrgUnit tries the stable id first, then falls back to
// display_name. Returns (uuid.Nil, false, nil) when neither
// resolves; the caller decides whether to skip or just leave the
// FK null on the resulting Employment.
func resolveOrgUnit(
	ctx context.Context, tx bun.IDB, lookup org_units.Lookup,
	ouID, ouName string,
) (uuid.UUID, bool, error) {
	if ouID != "" {
		id, ok, err := lookup.GetIDByExternalID(ctx, tx, ouID)
		if err != nil {
			return uuid.Nil, false, err
		}
		if ok {
			return id, true, nil
		}
	}
	if ouName != "" {
		id, ok, err := lookup.GetIDByDisplayName(ctx, tx, ouName)
		if err != nil {
			return uuid.Nil, false, err
		}
		if ok {
			return id, true, nil
		}
	}
	return uuid.Nil, false, nil
}

// upsertEmployment inserts or updates the Employment keyed by
// (person_id, code, start_date). Returns the resolved row id for
// downstream match writes.
func upsertEmployment(
	ctx context.Context, tx bun.IDB,
	personID uuid.UUID, code string,
	startDate time.Time, endDate *time.Time,
	orgUnitID uuid.UUID, titleName string,
	now time.Time,
) (uuid.UUID, error) {
	e := &employments.Employment{
		ID:          uuid.New(),
		PersonID:    personID,
		Code:        code,
		StartDate:   startDate,
		EndDate:     endDate,
		Description: stringPtr(titleName),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if orgUnitID != uuid.Nil {
		ouID := orgUnitID
		e.OrgUnitID = &ouID
	}
	var id uuid.UUID
	err := tx.NewInsert().Model(e).
		On("CONFLICT (person_id, code, start_date) DO UPDATE").
		Set("end_date    = EXCLUDED.end_date").
		Set("org_unit_id = EXCLUDED.org_unit_id").
		Set("description = EXCLUDED.description").
		Set("updated_at  = EXCLUDED.updated_at").
		Returning("id").
		Scan(ctx, &id)
	return id, err
}

// upsertEmploymentSidecars stores per-period scalars (title_id,
// title_name, org_unit_name when the FK didn't resolve) in the EAV
// employment_attributes sidecar.
func upsertEmploymentSidecars(
	ctx context.Context, tx bun.IDB,
	empID uuid.UUID, p parsedPeriod, ouResolved bool,
) error {
	if p.TitleID != "" {
		if err := upsertEmploymentAttribute(ctx, tx, empID, "title_id", p.TitleID); err != nil {
			return err
		}
	}
	if p.TitleName != "" {
		if err := upsertEmploymentAttribute(ctx, tx, empID, "title_name", p.TitleName); err != nil {
			return err
		}
	}
	if !ouResolved && p.OrgUnitName != "" {
		if err := upsertEmploymentAttribute(ctx, tx, empID, "org_unit_name", p.OrgUnitName); err != nil {
			return err
		}
	}
	return nil
}

func upsertEmploymentAttribute(ctx context.Context, tx bun.IDB, empID uuid.UUID, key, value string) error {
	attr := &employments.EmploymentAttribute{
		ID:           uuid.New(),
		EmploymentID: empID,
		Key:          key,
		Value:        value,
	}
	_, err := tx.NewInsert().Model(attr).
		On("CONFLICT (employment_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

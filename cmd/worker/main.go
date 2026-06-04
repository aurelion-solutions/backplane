// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command worker is a stand-alone orchestrator runner. It claims
// pending Pipeline Runs from Postgres (FOR UPDATE SKIP LOCKED),
// executes their steps via the in-process action registry, and writes
// status back through orchestrator.Service.
//
// Scale-out: run N processes; each opens E executor slots
// (AURELION_WORKER_SLOTS, default 4). Slots compete for the same
// pending queue — at-most-one delivery is enforced by SKIP LOCKED +
// the status-guarded UPDATE inside Service.ClaimPendingRun.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/actions/noop"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/runner"
	"github.com/aurelion-solutions/backplane/internal/core/postgres"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	normalize_access_grant "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/access_grant_record"
	normalize_account "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/account"
	normalize_employee "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/employee"
	normalize_orgunit "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/orgunit"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/actions/assess"
	cedarmech "github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/mechanisms/cedar"
	opamech "github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/mechanisms/opa"
	sodmech "github.com/aurelion-solutions/backplane/internal/engines/policy_assessment/mechanisms/sod"
	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/aurelion-solutions/backplane/internal/inventory/capability_grants"
	"github.com/aurelion-solutions/backplane/internal/inventory/capability_mappings"
	"github.com/aurelion-solutions/backplane/internal/inventory/consent"
	"github.com/aurelion-solutions/backplane/internal/inventory/employee_provider_mappings"
	"github.com/aurelion-solutions/backplane/internal/inventory/employment_record_matches"
	"github.com/aurelion-solutions/backplane/internal/inventory/employments"
	"github.com/aurelion-solutions/backplane/internal/inventory/evidence_chain"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	"github.com/aurelion-solutions/backplane/internal/inventory/org_units"
	"github.com/aurelion-solutions/backplane/internal/inventory/persons"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_assessment_runs"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
	"github.com/aurelion-solutions/backplane/internal/inventory/principals"
	"github.com/aurelion-solutions/backplane/internal/inventory/secrets"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/aurelion-solutions/backplane/internal/inventory/workload_lineage"
	"github.com/aurelion-solutions/backplane/internal/inventory/workloads"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/aurelion-solutions/backplane/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

const (
	logLevel           = "info"
	defaultWorkerSlots = 4
	storageProvider    = "file"
)

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("worker failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("worker — orchestrator runner node")
	fmt.Println()
	fmt.Println("  Claims pending Pipeline Runs from Postgres (FOR UPDATE SKIP LOCKED)")
	fmt.Println("  and executes their steps in process. Tune slot count via")
	fmt.Println("  AURELION_WORKER_SLOTS (default 4).")
	fmt.Println()
}

func run(log *slog.Logger) error {
	_ = godotenv.Load()

	providerName := envOr("AURELION_SECRET_PROVIDER", "file")
	secretsFile := envOr("AURELION_SECRETS_FILE", ".secrets.json")

	sf := secretmanagers.NewFactory()
	secretmanagers.RegisterFile(sf, secretsFile)
	secretmanagers.RegisterVault(sf)
	secretmanagers.RegisterOpenBao(sf)
	secretmanagers.RegisterAkeyless(sf)
	secretmanagers.RegisterConjur(sf)

	sm, err := sf.Get(providerName)
	if err != nil {
		return err
	}
	settings, err := config.Load(sm)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := postgres.New(ctx, postgres.Config{DSN: settings.Postgres.DSN(), Debug: settings.App.Debug})
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer func() { _ = db.Close() }()
	log.Info("postgres connected")

	mq, err := rabbitmq.New(rabbitmq.Config{
		URL: settings.RabbitMQ.URL(),
		Exchanges: []rabbitmq.Exchange{
			{Name: settings.RabbitMQ.LogsExchange, Type: rabbitmq.Topic},
			{Name: settings.RabbitMQ.EventsExchange, Type: rabbitmq.Topic},
		},
	})
	if err != nil {
		return err
	}
	defer func() { _ = mq.Close() }()
	log.Info("rabbitmq connected")

	bootSink := siem.NewMQSink(mq.Channel, settings.RabbitMQ.LogsExchange)
	siem.EmitInfo(ctx, bootSink, "worker", "worker started")
	defer siem.EmitInfo(context.Background(), bootSink, "worker", "worker stopping")

	eventsSink := events.NewMQ(mq.Channel, settings.RabbitMQ.EventsExchange)
	log.Info("events sink ready")

	// Cartridges + pipeline catalog.
	cf := cartridges.NewFactory()
	cartridges.RegisterFilesystem(cf, settings.Cartridges.Root)
	provider, err := cf.Get(settings.Cartridges.Provider)
	if err != nil {
		return err
	}
	log.Info("cartridges provider selected",
		slog.String("provider", settings.Cartridges.Provider),
		slog.String("root", settings.Cartridges.Root),
	)

	// Storage (data lake) — needed by inventory_normalize actions that
	// read raw records from lake batches by storage_key.
	stf := storage.NewFactory()
	storage.RegisterFile(stf, storage.DefaultBasePath)
	storage.RegisterS3(stf)
	storage.RegisterIceberg(stf)
	st, err := stf.Get(storageProvider)
	if err != nil {
		return err
	}
	log.Info("storage selected", slog.String("provider", storageProvider))

	// Action registry: noop (smoke / HITL test surface) +
	// inventory_normalize actions.
	reg := registry.New()
	noop.Register(reg)
	normalize_account.Register(reg, normalize_account.Deps{
		Lake: st,
		Repo: accounts.NewBunRepository(),
	})
	normalize_access_grant.Register(reg, normalize_access_grant.Deps{
		Lake:     st,
		Accounts: accounts.NewLookupBunRepository(),
		Mappings: capability_mappings.NewBunRepository(),
		Grants:   capability_grants.NewBunRepository(),
	})
	normalize_employee.Register(reg, normalize_employee.Deps{
		Lake:     st,
		Mappings: employee_provider_mappings.NewBunRepository(),
		Persons:  persons.NewAttributeLookupBunRepository(),
		OrgUnits: org_units.NewLookupBunRepository(),
		Matches:  employment_record_matches.NewBunRepository(),
	})
	normalize_orgunit.Register(reg, normalize_orgunit.Deps{Lake: st})

	// policy_assessment engine: store of cartridge-loaded policies +
	// dispatcher with the mechanism handlers this worker supports.
	policyStore := policy_assessment.NewStore()
	if n, err := policyStore.Reload(ctx, provider); err != nil {
		return fmt.Errorf("policy_assessment: store reload: %w", err)
	} else {
		log.Info("policy store loaded", slog.Int("entries", n))
	}
	policyDispatcher := policy_assessment.NewDispatcher()
	policyDispatcher.Register(opamech.New())
	policyDispatcher.Register(cedarmech.New())
	policyDispatcher.Register(sodmech.New())
	if ok, errs := policyDispatcher.PrepareAll(ctx, policyStore.All()); len(errs) > 0 {
		for _, e := range errs {
			log.Warn("policy prepare failed", slog.Any("err", e))
		}
		log.Info("policy prepare done", slog.Int("ok", ok), slog.Int("errors", len(errs)))
	} else {
		log.Info("policy prepare done", slog.Int("ok", ok))
	}

	// Lineage resolver + snapshot repo for the NHI assessment pass.
	wlsRepo := workloads.NewBunRepository(db)
	empsRepo := employments.NewBunRepository(db)
	personsRepo := persons.NewBunRepository(db)
	lineageRepo := workload_lineage.NewBunRepository(db)
	lineageResolver := workload_lineage.NewResolver(
		workerLineageWorkloadAdapter{repo: wlsRepo},
		workerLineageEmploymentAdapter{repo: empsRepo, orgUnits: org_units.NewBunRepository(db, nil)},
		workerLineagePersonAdapter{repo: personsRepo},
	)

	// policy_assessment.assess action — pipeline-runnable unit that
	// evaluates the active policy catalogue against the inventory
	// snapshot and persists findings.
	assess.Register(reg, assess.Deps{
		DB:             db,
		AccountsRepo:   accounts.NewBunRepository(),
		WorkloadsRepo:  wlsRepo,
		PrincipalsRepo: principals.NewBunRepository(db),
		SecretsPlain:   secrets.NewPlainBunRepository(),
		SecretsCert:    secrets.NewCertBunRepository(),
		ConsentApps:    consent.NewAppBunRepository(),
		ConsentGrants:  consent.NewGrantBunRepository(),
		OwnerTerminus: workerOwnerTerminusAdapter{
			principals: principals.NewBunRepository(db),
			emps:       empsRepo,
			resolver:   lineageResolver,
		},
		Lineage:      workerLineageResolverAdapter{resolver: lineageResolver},
		Snapshots:    workerSnapshotWriterAdapter{resolver: lineageResolver, repo: lineageRepo},
		RunsRepo:     policy_assessment_runs.NewBunRepository(db),
		FindingsRepo: findings.NewBunRepository(db),
		OutcomesSvc:  policy_evaluation_outcomes.NewService(policy_evaluation_outcomes.NewBunRepository(db)),
		EvidenceSvc:  evidence_chain.NewService(evidence_chain.NewBunRepository(db)),
		Store:        policyStore,
		Dispatcher:   policyDispatcher,
	})

	log.Info("action registry ready", slog.Int("actions", len(reg.All())))

	pipelineLoader := &loader.Loader{Actions: nil} // see backplane main for rationale
	catalog, err := orchestrator.LoadFromCartridges(provider, pipelineLoader, nil)
	if err != nil {
		return fmt.Errorf("orchestrator: load pipelines: %w", err)
	}
	log.Info("pipeline catalog loaded",
		slog.Int("pipelines", len(catalog.All())),
		slog.Any("cartridges", catalog.Sources()),
	)

	svc := orchestrator.NewService(orchestrator.NewBunRepository())

	slots := envInt("AURELION_WORKER_SLOTS", defaultWorkerSlots)
	tags := envTags("AURELION_WORKER_TAGS")
	log.Info("starting worker slots",
		slog.Int("slots", slots),
		slog.Any("tags", tags),
	)

	var wg sync.WaitGroup

	// Cartridge mtime watcher — rebuilds the pipeline catalog when
	// any cartridge YAML changes on disk. 5 s polling, same baseline
	// as the backplane sync loop.
	wg.Add(1)
	go func() {
		defer wg.Done()
		orchestrator.RunCatalogWatcher(
			ctx,
			catalog,
			provider,
			pipelineLoader,
			nil,
			settings.Cartridges.Root,
			cartridges.DefaultPollInterval,
			log.With(slog.String("component", "catalog_watcher")),
		)
	}()

	for i := 0; i < slots; i++ {
		wid := runner.NewWorkerIdentity(i, tags)
		r := runner.New(db, svc, reg, catalog, log.With(slog.Int("slot", i)), eventsSink, wid)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.WorkLoop(ctx); err != nil {
				log.Error("work loop terminated", slog.Any("err", err))
			}
		}()
	}

	<-ctx.Done()
	log.Info("shutdown signal received — waiting for slots to drain")

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelDrain()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Info("worker stopped cleanly")
	case <-drainCtx.Done():
		log.Warn("worker drain timeout — exiting (reclaim sweep will pick up stragglers)")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// envTags parses a CSV env var (e.g. "gpu,llm,prod") into a deduped,
// trimmed slice of non-empty entries. Returns nil for unset/empty
// vars — runner.NewWorkerIdentity makes the defensive copy.
func envTags(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range strings.Split(v, ",") {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// -------- workload_lineage port adapters --------

// workerLineageWorkloadAdapter maps workloads.Repository → workload_lineage.WorkloadReader.
type workerLineageWorkloadAdapter struct{ repo workloads.Repository }

func (a workerLineageWorkloadAdapter) GetByID(ctx context.Context, id uuid.UUID) (*workload_lineage.WorkloadRef, error) {
	w, err := a.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, workloads.ErrNotFound) {
			return nil, workload_lineage.ErrReaderNotFound
		}
		return nil, err
	}
	return &workload_lineage.WorkloadRef{
		ID:                w.ID,
		ExternalID:        w.ExternalID,
		Name:              w.Name,
		OwnerEmploymentID: w.OwnerEmploymentID,
	}, nil
}

// workerLineageEmploymentAdapter maps employments.Repository → workload_lineage.EmploymentReader.
type workerLineageEmploymentAdapter struct {
	repo     employments.Repository
	orgUnits org_units.Repository
}

func (a workerLineageEmploymentAdapter) GetByID(ctx context.Context, id uuid.UUID) (*workload_lineage.EmploymentRef, error) {
	e, err := a.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, employments.ErrNotFound) {
			return nil, workload_lineage.ErrReaderNotFound
		}
		return nil, err
	}
	return &workload_lineage.EmploymentRef{
		ID:        e.ID,
		PersonID:  e.PersonID,
		Code:      e.Code,
		StartDate: e.StartDate,
		EndDate:   e.EndDate,
		Title:     derefStr(e.Description),
		OrgUnit:   a.orgUnitName(ctx, e.OrgUnitID),
	}, nil
}

func (a workerLineageEmploymentAdapter) ListByPerson(ctx context.Context, personID uuid.UUID) ([]*workload_lineage.EmploymentRef, error) {
	emps, err := a.repo.ListByPerson(ctx, personID)
	if err != nil {
		return nil, err
	}
	out := make([]*workload_lineage.EmploymentRef, len(emps))
	for i, e := range emps {
		out[i] = &workload_lineage.EmploymentRef{
			ID:        e.ID,
			PersonID:  e.PersonID,
			Code:      e.Code,
			StartDate: e.StartDate,
			EndDate:   e.EndDate,
			Title:     derefStr(e.Description),
		}
	}
	return out, nil
}

// orgUnitName resolves an org-unit id to its display name, best-effort.
func (a workerLineageEmploymentAdapter) orgUnitName(ctx context.Context, id *uuid.UUID) string {
	if id == nil || a.orgUnits == nil {
		return ""
	}
	u, err := a.orgUnits.GetByID(ctx, *id)
	if err != nil || u == nil {
		return ""
	}
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Name
}

// derefStr returns the pointed-to string, or "" for a nil pointer.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// workerLineagePersonAdapter maps persons.Repository → workload_lineage.PersonReader.
type workerLineagePersonAdapter struct{ repo persons.Repository }

func (a workerLineagePersonAdapter) GetByID(ctx context.Context, id uuid.UUID) (*workload_lineage.PersonRef, error) {
	p, err := a.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, persons.ErrNotFound) {
			return nil, workload_lineage.ErrReaderNotFound
		}
		return nil, err
	}
	return &workload_lineage.PersonRef{
		ID:         p.ID,
		ExternalID: p.ExternalID,
		FullName:   p.FullName,
	}, nil
}

// workerLineageResolverAdapter maps workload_lineage.Resolver → assess.LineageResolver.
// It adapts the full OwnershipChain to assess's local ResolvedChain view.
// workerOwnerTerminusAdapter resolves a secret owner's terminus for the
// assess secret pass: workload principals route through the lineage
// resolver; employment principals are terminated when the person holds
// no active employment.
type workerOwnerTerminusAdapter struct {
	principals principals.Repository
	emps       employments.Repository
	resolver   *workload_lineage.Resolver
}

func (a workerOwnerTerminusAdapter) Resolve(ctx context.Context, principalID uuid.UUID) (string, error) {
	p, err := a.principals.GetByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principals.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	switch p.Kind {
	case shared.PrincipalKindWorkload:
		if p.PrincipalWorkloadID == nil {
			return "", nil
		}
		chain, rerr := a.resolver.Resolve(ctx, *p.PrincipalWorkloadID)
		if rerr != nil {
			return "", rerr
		}
		return chain.Terminus, nil
	case shared.PrincipalKindEmployment:
		if p.PrincipalEmploymentID == nil {
			return "", nil
		}
		emp, eerr := a.emps.GetByID(ctx, *p.PrincipalEmploymentID)
		if eerr != nil {
			return "", eerr
		}
		all, aerr := a.emps.ListByPerson(ctx, emp.PersonID)
		if aerr != nil {
			return "", aerr
		}
		active, cerr := a.emps.ListActiveByPerson(ctx, emp.PersonID, time.Now().UTC())
		if cerr != nil {
			return "", cerr
		}
		if len(all) > 0 && len(active) == 0 {
			return "terminated_human", nil
		}
		return "active_human", nil
	}
	return "", nil
}

type workerLineageResolverAdapter struct{ resolver *workload_lineage.Resolver }

func (a workerLineageResolverAdapter) Resolve(ctx context.Context, workloadID uuid.UUID) (assess.ResolvedChain, error) {
	chain, err := a.resolver.Resolve(ctx, workloadID)
	if err != nil {
		return assess.ResolvedChain{}, err
	}
	rc := assess.ResolvedChain{
		WorkloadID: chain.WorkloadID,
		Terminus:   chain.Terminus,
	}
	// Extract owner details from the person link if present.
	for _, l := range chain.Links {
		if l.Kind == "person" {
			rc.OwnerPersonID = l.RefID
			rc.OwnerLabel = l.Label
			rc.LastTerminationDate = l.EndDate
		}
	}
	return rc, nil
}

// workerSnapshotWriterAdapter satisfies assess.SnapshotWriter by re-resolving
// the full OwnershipChain and persisting it via the lineage repository.
// This is the ONLY path that writes snapshots (R1).
type workerSnapshotWriterAdapter struct {
	resolver *workload_lineage.Resolver
	repo     *workload_lineage.BunRepository
}

func (a workerSnapshotWriterAdapter) RecordSnapshot(ctx context.Context, workloadID uuid.UUID) error {
	chain, err := a.resolver.Resolve(ctx, workloadID)
	if err != nil {
		return err
	}
	return a.repo.RecordSnapshot(ctx, chain)
}

// Suppress unused-import warning when one of the imports isn't yet
// reached on a particular code path during refactors.
var _ = errors.New

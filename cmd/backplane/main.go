// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command backplane is the single composition root for the
// aurelion-backplane service. Wiring order:
//
//	envvars → secretmanagers.Factory → secretmanagers.Manager → config.Settings →
//	logger → postgres.DB → rabbitmq.Conn → events sink → storage →
//	siem → llm → connector RPC client → registration consumer →
//	integrations + inventory services → webserver → serve.
//
// Each factory fails fast: an unreachable dependency at startup aborts
// the boot with a non-zero exit. Hexagonal-style: domain packages
// receive their infra dependencies through constructor functions
// called from here, not via globals.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/descriptor"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/actions/noop"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/beat"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/matcher"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	core_pipelines "github.com/aurelion-solutions/backplane/internal/core/pipelines"
	core_policies "github.com/aurelion-solutions/backplane/internal/core/policies"
	"github.com/aurelion-solutions/backplane/internal/core/postgres"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/core/webserver"
	"github.com/aurelion-solutions/backplane/internal/engines/access_generate"
	access_generate_run "github.com/aurelion-solutions/backplane/internal/engines/access_generate/actions/run"
	"github.com/aurelion-solutions/backplane/internal/engines/inventory_discover"
	"github.com/aurelion-solutions/backplane/internal/engines/inventory_import"
	"github.com/aurelion-solutions/backplane/internal/engines/inventory_ingest"
	normalize_access_grant "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/access_grant_record"
	normalize_account "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/account"
	normalize_employee "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/employee"
	normalize_orgunit "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/orgunit"
	normalize_person "github.com/aurelion-solutions/backplane/internal/engines/inventory_normalize/actions/person"
	"github.com/aurelion-solutions/backplane/internal/integrations/applications"
	"github.com/aurelion-solutions/backplane/internal/integrations/connectors"
	"github.com/aurelion-solutions/backplane/internal/inventory/access_profile"
	"github.com/aurelion-solutions/backplane/internal/inventory/accounts"
	"github.com/aurelion-solutions/backplane/internal/inventory/capabilities"
	"github.com/aurelion-solutions/backplane/internal/inventory/capability_grants"
	"github.com/aurelion-solutions/backplane/internal/inventory/capability_mappings"
	"github.com/aurelion-solutions/backplane/internal/inventory/consent"
	"github.com/aurelion-solutions/backplane/internal/inventory/customers"
	"github.com/aurelion-solutions/backplane/internal/inventory/employee_provider_mappings"
	"github.com/aurelion-solutions/backplane/internal/inventory/employee_records"
	"github.com/aurelion-solutions/backplane/internal/inventory/employment_record_matches"
	"github.com/aurelion-solutions/backplane/internal/inventory/employments"
	"github.com/aurelion-solutions/backplane/internal/inventory/evidence_chain"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	"github.com/aurelion-solutions/backplane/internal/inventory/initiatives"
	"github.com/aurelion-solutions/backplane/internal/inventory/org_units"
	"github.com/aurelion-solutions/backplane/internal/inventory/persons"
	inv_pipelines "github.com/aurelion-solutions/backplane/internal/inventory/pipelines"
	inv_policies "github.com/aurelion-solutions/backplane/internal/inventory/policies"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_assessment_runs"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
	"github.com/aurelion-solutions/backplane/internal/inventory/principals"
	"github.com/aurelion-solutions/backplane/internal/inventory/secrets"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/aurelion-solutions/backplane/internal/inventory/workload_lineage"
	"github.com/aurelion-solutions/backplane/internal/inventory/workloads"
	"github.com/aurelion-solutions/backplane/internal/platform/llm"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/aurelion-solutions/backplane/internal/platform/storage"
	"github.com/aurelion-solutions/backplane/internal/transports/ingest_mq"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/uptrace/bun"
)

const (
	httpAddr        = ":8000"
	logLevel        = "info"
	storageProvider = "file"
	llmProvider     = "llamacpp"
)

var siemProviders = []string{"file", "stdout"}

func main() {
	printBanner()
	log := logger.New(os.Stderr, logLevel)
	if err := run(log); err != nil {
		log.Error("startup failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func printBanner() {
	fmt.Println("backplane — Aurelion API composition root")
	fmt.Println()
	fmt.Printf("  HTTP listening on %s\n", httpAddr)
	fmt.Printf("  curl localhost%s/healthz\n", httpAddr)
	fmt.Printf("  curl localhost%s/api/v0/applications\n", httpAddr)
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
	log.Info("config loaded",
		slog.String("postgres_host", settings.Postgres.Host),
		slog.String("rabbitmq_host", settings.RabbitMQ.Host),
		slog.Bool("debug", settings.App.Debug),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pgCfg := postgres.Config{DSN: settings.Postgres.DSN(), Debug: settings.App.Debug}
	var db *bun.DB
	for attempt := 1; ; attempt++ {
		db, err = postgres.New(ctx, pgCfg)
		if err == nil {
			break
		}
		log.Warn("postgres connect failed; retrying", slog.Int("attempt", attempt), slog.Any("err", err))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	defer func() { _ = db.Close() }()
	log.Info("postgres connected")

	mqCfg := rabbitmq.Config{
		URL: settings.RabbitMQ.URL(),
		Exchanges: []rabbitmq.Exchange{
			{Name: settings.RabbitMQ.EventsExchange, Type: rabbitmq.Topic},
			{Name: settings.RabbitMQ.LogsExchange, Type: rabbitmq.Topic},
			{Name: settings.RabbitMQ.ConnectorCommandsExchange, Type: rabbitmq.Direct},
			{Name: settings.RabbitMQ.ConnectorResponsesExchange, Type: rabbitmq.Direct},
			{Name: settings.RabbitMQ.ConnectorRegistrationExchange, Type: rabbitmq.Topic},
			{Name: ingest_mq.DefaultExchange, Type: rabbitmq.Topic},
		},
	}
	var mq *rabbitmq.Conn
	for attempt := 1; ; attempt++ {
		mq, err = rabbitmq.New(mqCfg)
		if err == nil {
			break
		}
		log.Warn("rabbitmq connect failed; retrying", slog.Int("attempt", attempt), slog.Any("err", err))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	defer func() { _ = mq.Close() }()
	log.Info("rabbitmq connected")

	bootSink := siem.NewMQSink(mq.Channel, settings.RabbitMQ.LogsExchange)
	siem.EmitInfo(ctx, bootSink, "backplane", "backplane started")
	defer siem.EmitInfo(context.Background(), bootSink, "backplane", "backplane stopping")

	eventsSink := events.NewMQ(mq.Channel, settings.RabbitMQ.EventsExchange)
	log.Info("events sink ready")

	stf := storage.NewFactory()
	storage.RegisterFile(stf, storage.DefaultBasePath)
	storage.RegisterS3(stf)
	storage.RegisterIceberg(stf)
	st, err := stf.Get(storageProvider)
	if err != nil {
		return err
	}
	log.Info("storage selected", slog.String("provider", storageProvider))

	lsf := siem.NewFactory()
	siem.RegisterFile(lsf, siem.DefaultFilePath)
	siem.RegisterStdout(lsf)
	siem.RegisterMQ(lsf, mq.Channel, settings.RabbitMQ.LogsExchange)
	siem.RegisterELK(lsf)
	siem.RegisterFluentd(lsf)
	siem.RegisterLoki(lsf)
	siem.RegisterNagios(lsf)
	siem.RegisterQRadar(lsf)
	siem.RegisterRsyslog(lsf)
	siem.RegisterSeq(lsf)
	siem.RegisterSplunk(lsf)
	siem.RegisterZabbix(lsf)
	sinks := make([]siem.Sink, 0, len(siemProviders))
	for _, name := range siemProviders {
		s, err := lsf.Get(name)
		if err != nil {
			return err
		}
		sinks = append(sinks, s)
	}
	siemSink := siem.NewMulti(sinks...)
	_ = siemSink
	log.Info("siem selected", slog.Any("providers", siemProviders))

	cf := cartridges.NewFactory()
	cartridges.RegisterFilesystem(cf, settings.Cartridges.Root)
	cartridgesProvider, err := cf.Get(settings.Cartridges.Provider)
	if err != nil {
		return err
	}
	log.Info("cartridges provider selected",
		slog.String("provider", settings.Cartridges.Provider),
		slog.String("root", settings.Cartridges.Root),
	)

	actionReg := registry.New()
	noop.Register(actionReg)
	normalize_account.Register(actionReg, normalize_account.Deps{
		Lake: st,
		Repo: accounts.NewBunRepository(),
	})
	normalize_access_grant.Register(actionReg, normalize_access_grant.Deps{
		Lake:     st,
		Accounts: accounts.NewLookupBunRepository(),
		Mappings: capability_mappings.NewBunRepository(),
		Grants:   capability_grants.NewBunRepository(),
	})
	normalize_employee.Register(actionReg, normalize_employee.Deps{
		Lake:     st,
		Mappings: employee_provider_mappings.NewBunRepository(),
		Persons:  persons.NewAttributeLookupBunRepository(),
		OrgUnits: org_units.NewLookupBunRepository(),
		Matches:  employment_record_matches.NewBunRepository(),
	})
	normalize_orgunit.Register(actionReg, normalize_orgunit.Deps{Lake: st})
	normalize_person.Register(actionReg, normalize_person.Deps{Lake: st})
	log.Info("action registry ready", slog.Int("actions", len(actionReg.All())))

	// Action-ref validation is intentionally off while most engines
	// still live in aurelion-kernel — flipping Actions to actionReg
	// will be safe once the Go-side engine surface catches up.
	pipelineLoader := &loader.Loader{Actions: nil}
	catalog, err := orchestrator.LoadFromCartridges(cartridgesProvider, pipelineLoader, nil)
	if err != nil {
		return fmt.Errorf("orchestrator: load pipelines: %w", err)
	}
	log.Info("pipeline catalog loaded",
		slog.Int("pipelines", len(catalog.All())),
		slog.Any("cartridges", catalog.Sources()),
	)

	llf := llm.NewFactory()
	llm.RegisterLlamaCpp(llf)
	llm.RegisterAnthropic(llf)
	llm.RegisterOpenAI(llf)
	llmClient, err := llf.Get(llmProvider)
	if err != nil {
		return err
	}
	_ = llmClient
	log.Info("llm selected", slog.String("provider", llmProvider))

	rpc := rabbitmq.NewRPCClient(mq.Conn, rabbitmq.RPCClientConfig{
		ResponsesExchange: settings.RabbitMQ.ConnectorResponsesExchange,
	})
	if err := rpc.Start(ctx); err != nil {
		return fmt.Errorf("connector rpc start: %w", err)
	}
	defer func() { _ = rpc.Close() }()
	log.Info("connector rpc client started",
		slog.String("client_id", rpc.ClientID()),
		slog.String("responses_exchange", settings.RabbitMQ.ConnectorResponsesExchange),
	)

	lakeReader := lakeReaderAdapter{factory: stf}
	_ = connectors.NewRPCClient(rpc, lakeReader, settings.RabbitMQ.ConnectorCommandsExchange)

	// Integrations -----------------------------------------------------
	appsRepo := applications.NewBunRepository(db)
	appsSvc := applications.NewService(appsRepo, eventsSink, nil)

	connRepo := connectors.NewBunRepository(db)
	connSvc := connectors.NewService(connRepo, nil)

	// Inventory --------------------------------------------------------
	personsRepo := persons.NewBunRepository(db)
	personsSvc := persons.NewService(personsRepo, eventsSink, nil)

	orgUnitsRepo := org_units.NewBunRepository(db, nil)
	orgUnitsSvc := org_units.NewService(orgUnitsRepo, eventsSink, nil, nil)

	empsRepo := employments.NewBunRepository(db)
	wlsRepo := workloads.NewBunRepository(db)
	custRepo := customers.NewBunRepository(db, nil)
	principalsRepo := principals.NewBunRepository(db)
	erRepo := employee_records.NewBunRepository(db)

	// Principals (subjects-replacement) depends on the body probes.
	principalsSvc := principals.NewService(principals.Deps{
		Repo: principalsRepo,
		Sink: eventsSink,
		Sources: principals.BodySources{
			Employments: principalEmploymentAdapter{repo: empsRepo},
			Workloads:   principalWorkloadAdapter{repo: wlsRepo},
			Customers:   principalCustomerAdapter{repo: custRepo},
		},
	})

	personsBridge := personsAdapter{repo: personsRepo}
	orgUnitsBridge := orgUnitsAdapter{repo: orgUnitsRepo}
	principalRecomputer := principalRecomputerAdapter{svc: principalsSvc}

	empsSvc := employments.NewService(employments.Deps{
		Repo:            empsRepo,
		Sink:            eventsSink,
		Persons:         personsBridge,
		OrgUnits:        orgUnitsBridge,
		Recomputer:      principalRecomputer,
		PersonResolver:  personsBridge,
		OrgUnitResolver: orgUnitsBridge,
	})

	wlsSvc := workloads.NewService(workloads.Deps{
		Repo:        wlsRepo,
		Sink:        eventsSink,
		Employments: workloadEmploymentAdapter{repo: empsRepo},
		Apps:        applicationCheckerAdapter{repo: appsRepo},
	})

	custSvc := customers.NewService(customers.Deps{
		Repo:       custRepo,
		Sink:       eventsSink,
		Recomputer: principalRecomputer,
	})

	personAPI := personAPIAdapter{
		personsRepo: personsRepo,
		personsSvc:  personsSvc,
		empsSvc:     empsSvc,
		empsRepo:    empsRepo,
	}
	resolver := employee_records.NewResolver(erRepo, personAPI)
	erSvc := employee_records.NewService(employee_records.Deps{
		Repo:         erRepo,
		Sink:         eventsSink,
		Apps:         applicationCheckerAdapter{repo: appsRepo},
		Persons:      erPersonsAdapter{repo: personsRepo},
		Employments:  erEmploymentsAdapter{repo: empsRepo},
		AppsResolver: applicationCodeResolver{repo: appsRepo},
		Resolver:     resolver,
	})

	// Engines ----------------------------------------------------------
	ingestRepo := inventory_ingest.NewBunRepository(db)
	ingestSvc := inventory_ingest.NewService(inventory_ingest.Deps{
		Repo: ingestRepo,
		Lake: ingestLakeAdapter{storage: st},
		Sink: eventsSink,
	})

	discoverDispatchChan, err := mq.Conn.Channel()
	if err != nil {
		return fmt.Errorf("discover dispatch channel: %w", err)
	}
	defer func() { _ = discoverDispatchChan.Close() }()

	discoverRepo := inventory_discover.NewBunRepository(db)
	discoverSvc := inventory_discover.NewService(inventory_discover.Deps{
		Repo:     discoverRepo,
		Dispatch: discoverDispatchAdapter{channel: discoverDispatchChan, exchange: settings.RabbitMQ.ConnectorCommandsExchange},
		Sink:     eventsSink,
	})

	// access_generate engine + run action -----------------------------
	capabilitiesRepo := capabilities.NewBunRepository(db)
	initiativesRepo := initiatives.NewBunRepository()
	accessGenEngine, err := access_generate.New(access_generate.Deps{
		Cartridges:   cartridgesProvider,
		BundleRef:    cartridges.Ref{ID: "popular"},
		Initiatives:  initiativesRepo,
		Accounts:     accounts.NewBunRepository(),
		Principals:   principalsRepo,
		Employments:  empsRepo,
		OrgUnits:     orgUnitsRepo,
		Applications: appsRepo,
		Capabilities: capabilitiesRepo,
		DB:           db,
		Events:       eventsSink,
		Actor:        "engine:access_generate",
	})
	if err != nil {
		return fmt.Errorf("access_generate engine: %w", err)
	}
	access_generate_run.Register(actionReg, access_generate_run.Deps{Engine: accessGenEngine})

	// Connector registration consumer goroutine.
	regChan, err := mq.Conn.Channel()
	if err != nil {
		return fmt.Errorf("registration consumer channel: %w", err)
	}
	defer func() { _ = regChan.Close() }()
	go func() {
		err := connectors.RunRegistrationConsumer(ctx, log, connSvc, connectors.RegistrationConsumerConfig{
			Channel:     regChan,
			Exchange:    settings.RabbitMQ.ConnectorRegistrationExchange,
			Queue:       settings.RabbitMQ.ConnectorRegistrationQueue,
			BindingKeys: []string{"connector.*"},
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("registration consumer terminated", slog.Any("err", err))
		}
	}()
	log.Info("registration consumer running",
		slog.String("exchange", settings.RabbitMQ.ConnectorRegistrationExchange),
		slog.String("queue", settings.RabbitMQ.ConnectorRegistrationQueue),
	)

	e := webserver.New(webserver.Config{
		Debug:            settings.App.Debug,
		CORSAllowOrigins: settings.App.CORSAllowOrigins,
	}, log)

	orchSvc := orchestrator.NewService(orchestrator.NewBunRepository())

	// Beat goroutine — periodic schedule firing + waiter timeout sweep.
	// Multi-replica safety is enforced inside Tick via pg_try_advisory_lock,
	// so it is safe to launch from every backplane process.
	go func() {
		b := beat.New(db, orchSvc, catalog, log.With(slog.String("component", "beat")))
		if err := b.Loop(ctx); err != nil {
			log.Error("beat loop terminated", slog.Any("err", err))
		}
	}()

	// Matcher goroutine — RabbitMQ event consumer. Cluster-wide there
	// is at most one active matcher; the rest become warm standbys via
	// a session-level pg_advisory_lock on a dedicated PG connection.
	matcherChan, err := mq.Conn.Channel()
	if err != nil {
		return fmt.Errorf("matcher channel: %w", err)
	}
	defer func() { _ = matcherChan.Close() }()
	go func() {
		mt := matcher.New(matcher.Config{
			DB:             db,
			Channel:        matcherChan,
			Service:        orchSvc,
			Catalog:        catalog,
			Log:            log.With(slog.String("component", "matcher")),
			EventsExchange: settings.RabbitMQ.EventsExchange,
			MatcherQueue:   settings.RabbitMQ.MatcherQueue,
		})
		if err := mt.Loop(ctx); err != nil {
			log.Error("matcher loop terminated", slog.Any("err", err))
		}
	}()

	// Discover subscriber — listens for connector.discover.* events
	// and walks the matching DiscoverRun through dispatched → running
	// → completed / failed.
	discoverSubChan, err := mq.Conn.Channel()
	if err != nil {
		return fmt.Errorf("discover subscriber channel: %w", err)
	}
	defer func() { _ = discoverSubChan.Close() }()
	go func() {
		err := inventory_discover.RunSubscriber(ctx, inventory_discover.SubscriberConfig{
			Channel:        discoverSubChan,
			EventsExchange: settings.RabbitMQ.EventsExchange,
			Queue:          inventory_discover.DefaultSubscriberQueue,
			Service:        discoverSvc,
			Log:            log.With(slog.String("component", "discover/subscriber")),
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("discover subscriber terminated", slog.Any("err", err))
		}
	}()
	log.Info("discover subscriber running",
		slog.String("exchange", settings.RabbitMQ.EventsExchange),
		slog.String("queue", inventory_discover.DefaultSubscriberQueue),
	)

	// Cartridge mtime watcher — rebuilds the in-memory pipeline
	// catalog when any cartridge YAML changes on disk. Backplane uses
	// the catalog for beat schedule firing and matcher routing.
	go orchestrator.RunCatalogWatcher(
		ctx,
		catalog,
		cartridgesProvider,
		pipelineLoader,
		nil,
		settings.Cartridges.Root,
		cartridges.DefaultPollInterval,
		log.With(slog.String("component", "catalog_watcher")),
	)

	// Cartridge → PG mirror sync loops. Cluster-wide singletons via
	// pg_try_advisory_lock so every backplane replica can tick safely.
	policiesRepo := inv_policies.NewBunRepository(db)
	pipelinesRepo := inv_pipelines.NewBunRepository(db)
	assessmentRunsRepo := policy_assessment_runs.NewBunRepository(db)
	findingsRepo := findings.NewBunRepository(db)
	policyOutcomesRepo := policy_evaluation_outcomes.NewBunRepository(db)
	evidenceChainRepo := evidence_chain.NewBunRepository(db)

	policiesSync := core_policies.New(core_policies.Deps{
		Provider: cartridgesProvider,
		Repo:     policiesRepo,
		Log:      log.With(slog.String("component", "policies-sync")),
	})
	go func() {
		if err := policiesSync.RunSyncLoop(ctx, db, core_policies.DefaultSyncInterval); err != nil {
			log.Error("policies sync loop terminated", slog.Any("err", err))
		}
	}()

	pipelinesSync := core_pipelines.New(core_pipelines.Deps{
		Provider: cartridgesProvider,
		Loader:   pipelineLoader,
		Repo:     pipelinesRepo,
		Log:      log.With(slog.String("component", "pipelines-sync")),
	})
	go func() {
		if err := pipelinesSync.RunSyncLoop(ctx, db, core_pipelines.DefaultSyncInterval); err != nil {
			log.Error("pipelines sync loop terminated", slog.Any("err", err))
		}
	}()

	apiV0 := e.Group("/api/v0")
	cartridges.RegisterRoutes(apiV0, cartridgesProvider)
	descriptor.RegisterRoutes(apiV0, cartridgesProvider)
	orchestrator.RegisterDefinitionRoutes(apiV0, catalog, actionReg)
	orchestrator.RegisterRunRoutes(apiV0, db, orchSvc, catalog)
	orchestrator.RegisterWorkerRoutes(apiV0, db, orchSvc)
	inv_policies.RegisterRoutes(apiV0, policiesRepo)
	inv_pipelines.RegisterRoutes(apiV0, pipelinesRepo)
	policy_assessment_runs.RegisterRoutes(apiV0, assessmentRunsRepo)
	findings.RegisterRoutes(apiV0, findingsRepo)
	policy_evaluation_outcomes.RegisterRoutes(apiV0, policyOutcomesRepo)
	evidence_chain.RegisterRoutes(apiV0, evidenceChainRepo)
	orchestrator.RegisterWellKnownRoutes(e, actionReg)
	applications.RegisterRoutes(apiV0, appsSvc, matchingAdapter{svc: connSvc})
	connectors.RegisterRoutes(apiV0, connSvc)
	persons.RegisterRoutes(apiV0, personsSvc)
	accounts.RegisterRoutes(apiV0, db, accounts.NewBunRepository(), accounts.NewLookupBunRepository())
	secrets.RegisterRoutes(apiV0, db,
		secrets.NewPlainBunRepository(), secrets.NewPlainBunRepository(),
		secrets.NewCertBunRepository(), secrets.NewCertBunRepository())
	consent.RegisterRoutes(apiV0, db,
		consent.NewAppBunRepository(), consent.NewAppBunRepository(),
		consent.NewGrantBunRepository(), consent.NewGrantBunRepository())
	access_profile.RegisterRoutes(apiV0, access_profile.NewService(access_profile.NewBunRepository(db)))
	org_units.RegisterRoutes(apiV0, orgUnitsSvc)
	employments.RegisterRoutes(apiV0, empsSvc)
	workloads.RegisterRoutes(apiV0, wlsSvc)
	// Lineage resolver: read-only; NO snapshot writer passed here (R1).
	lineageResolver := workload_lineage.NewResolver(
		wlLineageWorkloadAdapter{repo: wlsRepo},
		wlLineageEmploymentAdapter{repo: empsRepo},
		wlLineagePersonAdapter{repo: personsRepo},
	)
	workload_lineage.RegisterRoutes(apiV0, lineageResolver)
	customers.RegisterRoutes(apiV0, custSvc)
	employee_records.RegisterRoutes(apiV0, erSvc)
	principals.RegisterRoutes(apiV0, principalsSvc)
	inventory_ingest.RegisterRoutes(apiV0, ingestSvc)
	inventory_discover.RegisterRoutes(apiV0, discoverSvc)

	importSvc, err := inventory_import.NewService(inventory_import.Deps{
		Ingest:  ingestSvc,
		Actions: actionReg,
		DB:      db,
		Log:     log,
	})
	if err != nil {
		return fmt.Errorf("inventory_import: %w", err)
	}
	inventory_import.RegisterRoutes(apiV0, importSvc)

	serveErr := make(chan error, 1)
	go func() {
		log.Info("http listening", slog.String("addr", httpAddr))
		if err := e.Start(httpAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown error", slog.Any("err", err))
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// -------- Cross-slice adapters --------

// matchingAdapter bridges applications → connectors.
type matchingAdapter struct{ svc *connectors.Service }

func (m matchingAdapter) MatchingForTags(ctx context.Context, requiredTags []string, onlineOnly bool) (any, error) {
	insts, err := m.svc.MatchingForTags(ctx, requiredTags, onlineOnly)
	if err != nil {
		return nil, err
	}
	out := make([]connectors.InstanceWire, 0, len(insts))
	for _, inst := range insts {
		out = append(out, connectors.NewInstanceWire(inst))
	}
	return out, nil
}

// ingestLakeAdapter implements inventory_ingest.Lake by delegating
// to the configured platform/storage backend.
type ingestLakeAdapter struct{ storage storage.Storage }

func (a ingestLakeAdapter) WriteBatch(ctx context.Context, datasetType string, records []map[string]any) (string, error) {
	return a.storage.WriteBatch(ctx, datasetType, records)
}

func (a ingestLakeAdapter) AntiJoin(ctx context.Context, datasetType string, candidates []storage.Candidate) (storage.AntiJoinResult, error) {
	return a.storage.AntiJoin(ctx, datasetType, candidates)
}

// discoverDispatchAdapter publishes one fire-and-forget discover
// command directly to the connector commands exchange. The connector
// signals progress later via its own MQ events; discover never blocks
// for an RPC reply.
type discoverDispatchAdapter struct {
	channel  *amqp.Channel
	exchange string
}

func (a discoverDispatchAdapter) Dispatch(ctx context.Context, cmd inventory_discover.Command) error {
	body, err := json.Marshal(map[string]any{
		"correlation_id": cmd.CorrelationID,
		"operation":      cmd.Operation,
		"dataset_type":   cmd.DatasetType,
		"payload":        cmd.Payload,
		"async":          true,
	})
	if err != nil {
		return fmt.Errorf("discover/dispatch: marshal: %w", err)
	}
	return a.channel.PublishWithContext(ctx, a.exchange, cmd.InstanceID, false, false, amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		CorrelationId: cmd.CorrelationID,
		Body:          body,
	})
}

// lakeReaderAdapter implements connectors.LakeReader.
type lakeReaderAdapter struct{ factory *storage.Factory }

func (a lakeReaderAdapter) ReadBatch(ctx context.Context, provider string, storageKey string) ([]map[string]any, error) {
	s, err := a.factory.Get(provider)
	if err != nil {
		return nil, err
	}
	return s.ReadBatch(ctx, storageKey)
}

// personsAdapter bridges persons-repo to the employments slice.
type personsAdapter struct{ repo persons.Repository }

func (a personsAdapter) PersonExists(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := a.repo.GetByID(ctx, id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, persons.ErrNotFound) {
		return false, nil
	}
	return false, err
}
func (a personsAdapter) PersonIDByExternalID(ctx context.Context, externalID string) (uuid.UUID, bool, error) {
	p, err := a.repo.GetByExternalID(ctx, externalID)
	if err == nil {
		return p.ID, true, nil
	}
	if errors.Is(err, persons.ErrNotFound) {
		return uuid.Nil, false, nil
	}
	return uuid.Nil, false, err
}

// orgUnitsAdapter bridges org_units-repo to the employments slice.
type orgUnitsAdapter struct{ repo org_units.Repository }

func (a orgUnitsAdapter) OrgUnitExists(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := a.repo.GetByID(ctx, id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, org_units.ErrNotFound) {
		return false, nil
	}
	return false, err
}
func (a orgUnitsAdapter) OrgUnitIDByExternalID(ctx context.Context, externalID string) (uuid.UUID, bool, error) {
	u, err := a.repo.GetByExternalID(ctx, externalID)
	if err == nil {
		return u.ID, true, nil
	}
	if errors.Is(err, org_units.ErrNotFound) {
		return uuid.Nil, false, nil
	}
	return uuid.Nil, false, err
}

// applicationCheckerAdapter is reused by workloads + employee_records.
type applicationCheckerAdapter struct{ repo applications.Repository }

func (a applicationCheckerAdapter) ApplicationExists(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := a.repo.GetByID(ctx, id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, applications.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// applicationCodeResolver lifts an application code → uuid for
// employee_records bulk upsert.
type applicationCodeResolver struct{ repo applications.Repository }

func (a applicationCodeResolver) ApplicationIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	app, err := a.repo.GetByCode(ctx, code)
	if err == nil {
		return app.ID, true, nil
	}
	if errors.Is(err, applications.ErrNotFound) {
		return uuid.Nil, false, nil
	}
	return uuid.Nil, false, err
}

// workloadEmploymentAdapter implements workloads.EmploymentChecker.
type workloadEmploymentAdapter struct{ repo employments.Repository }

func (a workloadEmploymentAdapter) EmploymentExists(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := a.repo.GetByID(ctx, id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, employments.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// principalRecomputerAdapter bridges employments/customers → principals.
type principalRecomputerAdapter struct{ svc *principals.Service }

func (a principalRecomputerAdapter) RecomputeForBody(ctx context.Context, kind shared.PrincipalKind, bodyID uuid.UUID) error {
	err := a.svc.RecomputeForBody(ctx, kind, bodyID)
	if err != nil && errors.Is(err, principals.ErrPrincipalMissingForBody) {
		// No principal row yet — recompute is a no-op until one is
		// created. Expected steady state during bulk imports that
		// haven't reached the principals pass yet.
		return nil
	}
	return err
}

// principalEmploymentAdapter / principalWorkloadAdapter /
// principalCustomerAdapter wire principals → underlying body state.
type principalEmploymentAdapter struct{ repo employments.Repository }

func (a principalEmploymentAdapter) EmploymentCode(ctx context.Context, id uuid.UUID) (string, bool, error) {
	e, err := a.repo.GetByID(ctx, id)
	if err == nil {
		return e.Code, true, nil
	}
	if errors.Is(err, employments.ErrNotFound) {
		return "", false, nil
	}
	return "", false, err
}

type principalWorkloadAdapter struct{ repo workloads.Repository }

func (a principalWorkloadAdapter) WorkloadExists(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := a.repo.GetByID(ctx, id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, workloads.ErrNotFound) {
		return false, nil
	}
	return false, err
}

type principalCustomerAdapter struct{ repo customers.Repository }

func (a principalCustomerAdapter) CustomerState(ctx context.Context, id uuid.UUID) (principals.CustomerStateView, error) {
	c, err := a.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, customers.ErrNotFound) {
			return principals.CustomerStateView{Exists: false}, nil
		}
		return principals.CustomerStateView{}, err
	}
	return principals.CustomerStateView{
		Exists:        true,
		EmailVerified: c.EmailVerified,
	}, nil
}

// workload_lineage port adapters — map slice repos to the narrow
// reader ports declared in internal/inventory/workload_lineage/ports.go.

type wlLineageWorkloadAdapter struct{ repo workloads.Repository }

func (a wlLineageWorkloadAdapter) GetByID(ctx context.Context, id uuid.UUID) (*workload_lineage.WorkloadRef, error) {
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

type wlLineageEmploymentAdapter struct{ repo employments.Repository }

func (a wlLineageEmploymentAdapter) GetByID(ctx context.Context, id uuid.UUID) (*workload_lineage.EmploymentRef, error) {
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
	}, nil
}

func (a wlLineageEmploymentAdapter) ListByPerson(ctx context.Context, personID uuid.UUID) ([]*workload_lineage.EmploymentRef, error) {
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
		}
	}
	return out, nil
}

type wlLineagePersonAdapter struct{ repo persons.Repository }

func (a wlLineagePersonAdapter) GetByID(ctx context.Context, id uuid.UUID) (*workload_lineage.PersonRef, error) {
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

// erPersonsAdapter / erEmploymentsAdapter are the employee_records
// service's PersonChecker / EmploymentChecker.
type erPersonsAdapter struct{ repo persons.Repository }

func (a erPersonsAdapter) PersonExists(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := a.repo.GetByID(ctx, id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, persons.ErrNotFound) {
		return false, nil
	}
	return false, err
}

type erEmploymentsAdapter struct{ repo employments.Repository }

func (a erEmploymentsAdapter) EmploymentExistsForPerson(ctx context.Context, empID, personID uuid.UUID) (bool, error) {
	e, err := a.repo.GetByID(ctx, empID)
	if err != nil {
		if errors.Is(err, employments.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return e.PersonID == personID, nil
}

// personAPIAdapter is what the employee_records Resolver dispatches to:
// it lifts (find / create / propagate / primary-employment) onto the
// concrete persons + employments services and repos.
type personAPIAdapter struct {
	personsRepo persons.Repository
	personsSvc  *persons.Service
	empsRepo    employments.Repository
	empsSvc     *employments.Service
}

func (a personAPIAdapter) FindPersonByAttribute(ctx context.Context, key, value string) (uuid.UUID, bool, error) {
	rows, _, err := a.personsRepo.List(ctx, 0, 0)
	if err != nil {
		return uuid.Nil, false, err
	}
	// O(N*M) scan is fine for now — when the inventory grows we will
	// add a dedicated repo method backed by an index on
	// person_attributes(key, value).
	for _, p := range rows {
		attrs, err := a.personsRepo.ListAttributes(ctx, p.ID)
		if err != nil {
			return uuid.Nil, false, err
		}
		for _, attr := range attrs {
			if attr.Key == key && attr.Value == value {
				return p.ID, true, nil
			}
		}
	}
	return uuid.Nil, false, nil
}

func (a personAPIAdapter) CreatePersonWithEmployment(ctx context.Context, key, value string) (uuid.UUID, uuid.UUID, error) {
	stamp := uuid.New().String()
	p, err := a.personsSvc.Create(ctx, persons.CreatePayload{
		ExternalID: "resolver-" + stamp,
		FullName:   "resolver-created",
	})
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("resolver: person: %w", err)
	}
	if _, err := a.personsSvc.AddAttribute(ctx, p.ID, persons.AttributeCreatePayload{
		Key: strings.TrimSpace(key), Value: value,
	}); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("resolver: seed person attribute: %w", err)
	}
	e, err := a.empsSvc.Create(ctx, employments.CreatePayload{
		PersonID:  p.ID,
		Code:      "active",
		StartDate: time.Now().UTC(),
	})
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("resolver: employment: %w", err)
	}
	return p.ID, e.ID, nil
}

func (a personAPIAdapter) PropagateAttribute(ctx context.Context, personID uuid.UUID, key, value string) error {
	_, err := a.personsSvc.AddAttribute(ctx, personID, persons.AttributeCreatePayload{
		Key: strings.TrimSpace(key), Value: value,
	})
	return err
}

func (a personAPIAdapter) PrimaryEmploymentForPerson(ctx context.Context, personID uuid.UUID) (uuid.UUID, bool, error) {
	active, err := a.empsRepo.ListActiveByPerson(ctx, personID, time.Now().UTC())
	if err != nil {
		return uuid.Nil, false, err
	}
	if len(active) == 0 {
		return uuid.Nil, false, nil
	}
	return active[0].ID, true, nil
}

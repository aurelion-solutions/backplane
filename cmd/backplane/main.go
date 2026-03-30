// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Command backplane is the single composition root for the
// aurelion-backplane service. Wiring order:
//
//	envvars → secret.Factory → secret.Manager → config.Settings →
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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/core/logger"
	"github.com/aurelion-solutions/backplane/internal/core/postgres"
	"github.com/aurelion-solutions/backplane/internal/core/rabbitmq"
	"github.com/aurelion-solutions/backplane/internal/core/webserver"
	"github.com/aurelion-solutions/backplane/internal/integrations/applications"
	"github.com/aurelion-solutions/backplane/internal/integrations/connectors"
	"github.com/aurelion-solutions/backplane/internal/inventory/customers"
	"github.com/aurelion-solutions/backplane/internal/inventory/employee_records"
	"github.com/aurelion-solutions/backplane/internal/inventory/employments"
	"github.com/aurelion-solutions/backplane/internal/inventory/org_units"
	"github.com/aurelion-solutions/backplane/internal/inventory/persons"
	"github.com/aurelion-solutions/backplane/internal/inventory/principals"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/aurelion-solutions/backplane/internal/inventory/workloads"
	"github.com/aurelion-solutions/backplane/internal/platform/llm"
	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
	"github.com/aurelion-solutions/backplane/internal/platform/siem"
	"github.com/aurelion-solutions/backplane/internal/platform/storage"
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
	_ = st
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
	connectorRPC := connectors.NewRPCClient(rpc, lakeReader, settings.RabbitMQ.ConnectorCommandsExchange)
	_ = connectorRPC

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

	apiV0 := e.Group("/api/v0")
	applications.RegisterRoutes(apiV0, appsSvc, matchingAdapter{svc: connSvc})
	connectors.RegisterRoutes(apiV0, connSvc)
	persons.RegisterRoutes(apiV0, personsSvc)
	org_units.RegisterRoutes(apiV0, orgUnitsSvc)
	employments.RegisterRoutes(apiV0, empsSvc)
	workloads.RegisterRoutes(apiV0, wlsSvc)
	customers.RegisterRoutes(apiV0, custSvc)
	employee_records.RegisterRoutes(apiV0, erSvc)
	principals.RegisterRoutes(apiV0, principalsSvc)

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

package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	alarmapp "microgrid-cloud/internal/alarms/application"
	alarms "microgrid-cloud/internal/alarms/domain"
	alarmrepo "microgrid-cloud/internal/alarms/infrastructure/postgres"
	alarminterfaces "microgrid-cloud/internal/alarms/interfaces"
	alarmhttp "microgrid-cloud/internal/alarms/interfaces/http"
	alarmnotify "microgrid-cloud/internal/alarms/notify"
	"microgrid-cloud/internal/analytics/application"
	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	appstatistic "microgrid-cloud/internal/analytics/application/statistic"
	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
	analyticsrepo "microgrid-cloud/internal/analytics/infrastructure/postgres"
	analyticsinterfaces "microgrid-cloud/internal/analytics/interfaces"
	apihttp "microgrid-cloud/internal/api/http"
	"microgrid-cloud/internal/audit"
	"microgrid-cloud/internal/auth"
	commandsapp "microgrid-cloud/internal/commands/application"
	commandsevents "microgrid-cloud/internal/commands/application/events"
	commandsrepo "microgrid-cloud/internal/commands/infrastructure/postgres"
	commandsinterfaces "microgrid-cloud/internal/commands/interfaces"
	commandshttp "microgrid-cloud/internal/commands/interfaces/http"
	"microgrid-cloud/internal/eventing"
	eventingrepo "microgrid-cloud/internal/eventing/infrastructure/postgres"
	masterdata "microgrid-cloud/internal/masterdata/domain"
	masterdatarepo "microgrid-cloud/internal/masterdata/infrastructure/postgres"
	"microgrid-cloud/internal/observability/metrics"
	provisioning "microgrid-cloud/internal/provisioning/application"
	provisioninghttp "microgrid-cloud/internal/provisioning/interfaces/http"
	settlementadapters "microgrid-cloud/internal/settlement/adapters/analytics"
	settlementapp "microgrid-cloud/internal/settlement/application"
	settlementrepo "microgrid-cloud/internal/settlement/infrastructure/postgres"
	settlementpricing "microgrid-cloud/internal/settlement/infrastructure/pricing"
	settlementinterfaces "microgrid-cloud/internal/settlement/interfaces"
	shadowapp "microgrid-cloud/internal/shadowrun/application"
	shadowrepo "microgrid-cloud/internal/shadowrun/infrastructure/postgres"
	shadowhttp "microgrid-cloud/internal/shadowrun/interfaces/http"
	shadowmetrics "microgrid-cloud/internal/shadowrun/metrics"
	shadownotify "microgrid-cloud/internal/shadowrun/notify"
	strategytelemetry "microgrid-cloud/internal/strategy/adapters/telemetry"
	strategyapp "microgrid-cloud/internal/strategy/application"
	strategyrepo "microgrid-cloud/internal/strategy/infrastructure/postgres"
	strategyhttp "microgrid-cloud/internal/strategy/interfaces/http"
	"microgrid-cloud/internal/tbadapter"
	telemetryadapters "microgrid-cloud/internal/telemetry/adapters/analytics"
	telemetryevents "microgrid-cloud/internal/telemetry/application/events"
	telemetrypostgres "microgrid-cloud/internal/telemetry/infrastructure/postgres"
	thingsboard "microgrid-cloud/internal/telemetry/interfaces/thingsboard"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg := loadConfig()
	logger := log.New(os.Stdout, "", log.LstdFlags)

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		logger.Fatalf("db open error: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		logger.Fatalf("db ping error: %v", err)
	}

	metrics.Init(db, logger)
	stationChecker := auth.NewStationChecker(db)
	auditRepo := audit.NewRepository(db)

	telemetryRepo := telemetrypostgres.NewTelemetryRepository(db)
	telemetryQuery := telemetrypostgres.NewTelemetryQuery(db)
	pointMappingRepo := masterdatarepo.NewPointMappingRepository(db)
	stationRepo := masterdatarepo.NewStationRepository(db)

	queryAdapter, err := telemetryadapters.NewQueryAdapter(cfg.TenantID, telemetryQuery, pointMappingRepo)
	if err != nil {
		logger.Fatalf("telemetry query adapter error: %v", err)
	}

	baseBus := eventbus.NewInMemoryBus()
	registry := eventing.NewRegistry()
	registry.Register(events.TelemetryWindowClosed{})
	registry.Register(events.StatisticCalculated{})
	registry.Register(settlementapp.SettlementCalculated{})
	registry.Register(commandsevents.CommandIssued{})
	registry.Register(commandsevents.CommandAcked{})
	registry.Register(commandsevents.CommandFailed{})
	registry.Register(telemetryevents.TelemetryReceived{})

	outboxStore := eventingrepo.NewOutboxStore(db)
	processedStore := eventingrepo.NewProcessedStore(db)
	dlqStore := eventingrepo.NewDLQStore(db)
	dispatcher := eventing.NewDispatcher(baseBus, outboxStore, registry, dlqStore)
	publisher := eventing.NewPublisher(outboxStore, dispatcher, cfg.TenantID, baseBus)
	bus := publisher
	statsRepo := analyticsrepo.NewPostgresStatisticRepository(db, cfg.StationID)

	hourlyService := application.NewHourlyStatisticAppService(
		statsRepo,
		queryAdapter,
		telemetryadapters.SumStatisticCalculator{},
		bus,
		hourStatisticIDFactory{},
		systemClock{},
	)

	rollupService, err := domainstatistic.NewDailyRollupService(statsRepo, domainstatistic.SystemClock{}, cfg.ExpectedHours)
	if err != nil {
		logger.Fatalf("daily rollup service error: %v", err)
	}
	dailyApp, err := appstatistic.NewDailyRollupAppService(rollupService, statsRepo, bus, domainstatistic.SystemClock{})
	if err != nil {
		logger.Fatalf("daily rollup app error: %v", err)
	}

	application.WireAnalyticsEventBus(baseBus, hourlyService, dailyApp, processedStore)
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[events.StatisticCalculated](), "analytics.log", func(ctx context.Context, event any) error {
		evt, ok := event.(events.StatisticCalculated)
		if !ok {
			return eventbus.ErrInvalidEventType
		}
		if evt.Granularity == domainstatistic.GranularityHour {
			logger.Printf("hour statistic calculated: station=%s id=%s period=%s", evt.StationID, evt.StatisticID, evt.PeriodStart.Format(time.RFC3339))
		}
		if evt.Granularity == domainstatistic.GranularityDay {
			logger.Printf("day statistic calculated: station=%s id=%s period=%s", evt.StationID, evt.StatisticID, evt.PeriodStart.Format(time.RFC3339))
		}
		return nil
	}, processedStore)

	dayEnergyReader := settlementadapters.NewDayHourEnergyReader(db, settlementadapters.WithExpectedHours(cfg.ExpectedHours))
	priceProvider, err := settlementpricing.NewFixedPriceProvider(cfg.PricePerKWh)
	if err != nil {
		logger.Fatalf("price provider error: %v", err)
	}
	settlementRepo := settlementrepo.NewSettlementRepository(db, settlementrepo.WithTenantID(cfg.TenantID), settlementrepo.WithCurrency(cfg.Currency))
	settlementPublisher := settlementinterfaces.NewOutboxPublisher(publisher, cfg.TenantID)
	settlementApp, err := settlementapp.NewDaySettlementApplicationService(settlementRepo, dayEnergyReader, priceProvider, settlementPublisher, systemClock{})
	if err != nil {
		logger.Fatalf("settlement app error: %v", err)
	}
	settlementHandler, err := settlementinterfaces.NewDayStatisticCalculatedHandler(settlementApp, logger)
	if err != nil {
		logger.Fatalf("settlement handler error: %v", err)
	}
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[events.StatisticCalculated](), "settlement.day", settlementHandler.HandleStatisticCalculated, processedStore)

	shadowCfg, err := shadowapp.LoadConfig()
	if err != nil {
		logger.Fatalf("shadowrun config error: %v", err)
	}
	shadowRepo := shadowrepo.NewRepository(db)

	alarmRuleRepo := alarmrepo.NewAlarmRuleRepository(db)
	alarmRepo := alarmrepo.NewAlarmRepository(db)
	alarmStateRepo := alarmrepo.NewAlarmRuleStateRepository(db)
	alarmBroker := alarmhttp.NewSSEBroker()
	alarmNotifiers := []alarmapp.AlarmNotifier{alarmBroker}
	if cfg.AlarmWebhookURL != "" {
		channel, err := alarmnotify.NewWebhookChannel(cfg.AlarmWebhookURL)
		if err != nil {
			logger.Fatalf("alarm webhook error: %v", err)
		}
		tpl, err := alarmnotify.NewTemplate(cfg.AlarmNotifyTemplate)
		if err != nil {
			logger.Fatalf("alarm template error: %v", err)
		}
		opts := []alarmnotify.Option{
			alarmnotify.WithEscalation(cfg.AlarmEscalationAfter),
			alarmnotify.WithCooldown(cfg.AlarmNotifyCooldown),
			alarmnotify.WithDedupeWindow(cfg.AlarmNotifyDedupeWindow),
			alarmnotify.WithRequestTimeout(cfg.AlarmNotifyTimeout),
		}
		if resolver := buildShadowrunReportResolver(shadowRepo, cfg.AlarmReportBaseURL, cfg.AlarmReportLookbackDays); resolver != nil {
			opts = append(opts, alarmnotify.WithReportURLResolver(resolver))
		}
		alarmNotifier, err := alarmnotify.NewNotifier(alarmRuleRepo, stationRepo, alarmRepo, channel, tpl, opts...)
		if err != nil {
			logger.Fatalf("alarm notifier error: %v", err)
		}
		alarmNotifiers = append(alarmNotifiers, alarmNotifier)
	}
	alarmService, err := alarmapp.NewService(alarmRuleRepo, alarmRepo, alarmStateRepo, pointMappingRepo, cfg.TenantID, alarmapp.WithNotifier(alarmnotify.NewMultiNotifier(alarmNotifiers...)))
	if err != nil {
		logger.Fatalf("alarm service error: %v", err)
	}
	alarmConsumer, err := alarminterfaces.NewTelemetryReceivedConsumer(alarmService)
	if err != nil {
		logger.Fatalf("alarm consumer error: %v", err)
	}
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[telemetryevents.TelemetryReceived](), "alarms.telemetry", func(ctx context.Context, event any) error {
		evt, ok := event.(telemetryevents.TelemetryReceived)
		if !ok {
			return eventbus.ErrInvalidEventType
		}
		return alarmConsumer.Consume(ctx, evt)
	}, processedStore)

	statementRepo := settlementrepo.NewStatementRepository(db)
	statementService, err := settlementapp.NewStatementService(statementRepo, cfg.TenantID)
	if err != nil {
		logger.Fatalf("statement service error: %v", err)
	}
	statementHandler, err := settlementinterfaces.NewStatementHandler(statementService, stationChecker, auditRepo)
	if err != nil {
		logger.Fatalf("statement handler error: %v", err)
	}

	ingestHandler, err := thingsboard.NewIngestHandler(telemetryRepo, publisher, logger)
	if err != nil {
		logger.Fatalf("ingest handler error: %v", err)
	}
	windowCloseHandler, err := analyticsinterfaces.NewWindowCloseHandler(publisher, logger)
	if err != nil {
		logger.Fatalf("window close handler error: %v", err)
	}

	tbClient, err := tbadapter.NewClient(cfg.TBBaseURL, cfg.TBToken)
	if err != nil {
		logger.Fatalf("tb client error: %v", err)
	}
	provisionService, err := provisioning.NewService(db, tbClient)
	if err != nil {
		logger.Fatalf("provisioning service error: %v", err)
	}
	provisionHandler, err := provisioninghttp.NewStationProvisioningHandler(provisionService, auditRepo)
	if err != nil {
		logger.Fatalf("provisioning handler error: %v", err)
	}

	commandRepo := commandsrepo.NewCommandRepository(db)
	commandService, err := commandsapp.NewService(commandRepo, publisher, cfg.TenantID)
	if err != nil {
		logger.Fatalf("command service error: %v", err)
	}
	commandHandler, err := commandshttp.NewHandler(commandService, stationChecker, auditRepo)
	if err != nil {
		logger.Fatalf("command handler error: %v", err)
	}
	commandConsumer, err := commandsinterfaces.NewTBRPCConsumer(commandRepo, tbClient, publisher, logger)
	if err != nil {
		logger.Fatalf("command consumer error: %v", err)
	}
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[commandsevents.CommandIssued](), "tb.rpc", commandConsumer.HandleCommandIssued, processedStore)

	strategyRepo := strategyrepo.NewRepository(db)
	strategyService, err := strategyapp.NewService(strategyRepo)
	if err != nil {
		logger.Fatalf("strategy service error: %v", err)
	}
	strategyHandler, err := strategyhttp.NewHandler(strategyService, stationChecker, auditRepo)
	if err != nil {
		logger.Fatalf("strategy handler error: %v", err)
	}
	strategyTelemetry := strategytelemetry.NewLatestReader(db)
	strategyEngine, err := strategyapp.NewEngine(strategyRepo, strategyTelemetry, commandService, cfg.TenantID)
	if err != nil {
		logger.Fatalf("strategy engine error: %v", err)
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for tick := range ticker.C {
			if err := strategyEngine.Tick(context.Background(), tick.UTC()); err != nil {
				logger.Printf("strategy tick error: %v", err)
			}
		}
	}()

	shadowMetrics := shadowmetrics.New()
	var shadowNotifier shadownotify.Notifier
	if shadowCfg.WebhookURL != "" {
		shadowNotifier = shadownotify.NewWebhookNotifier(shadowCfg.WebhookURL)
	}
	shadowRunner := shadowapp.NewRunner(shadowRepo, db, shadowCfg, shadowNotifier, shadowMetrics, logger)
	shadowHandler, err := shadowhttp.NewHandler(shadowRunner, shadowRepo, cfg.TenantID, stationChecker)
	if err != nil {
		logger.Fatalf("shadowrun handler error: %v", err)
	}
	shadowScheduler := shadowapp.NewScheduler(shadowRunner, cfg.TenantID, shadowCfg.Schedule.Stations, shadowCfg.Schedule.DailyAt, logger)
	go shadowScheduler.Start(context.Background())

	policy := auth.NewDefaultPolicy([]string{"/healthz", "/metrics"}, []string{"/ingest/"})
	authMiddleware := auth.NewMiddleware([]byte(cfg.JWTSecret), policy)
	ingestAuth := auth.NewIngestAuthMiddleware([]byte(cfg.IngestSecret), time.Duration(cfg.IngestSkewSeconds)*time.Second)

	mux := http.NewServeMux()
	mux.Handle("/ingest/thingsboard/telemetry", ingestAuth.Wrap(ingestHandler))
	mux.Handle("/analytics/window-close", windowCloseHandler)
	mux.Handle("/api/v1/provisioning/stations", provisionHandler)
	mux.Handle("/api/v1/commands", commandHandler)
	mux.Handle("/api/v1/strategies/", strategyHandler)
	mux.Handle("/api/v1/shadowrun/run", shadowHandler)
	mux.Handle("/api/v1/shadowrun/reports", shadowHandler)
	mux.Handle("/api/v1/shadowrun/reports/", shadowHandler)
	mux.Handle("/api/v1/stats", apihttp.NewStatsHandler(db, stationChecker))
	mux.Handle("/api/v1/settlements", apihttp.NewSettlementsHandler(db, cfg.TenantID, stationChecker))
	mux.Handle("/api/v1/statements", statementHandler)
	mux.Handle("/api/v1/statements/", statementHandler)
	mux.Handle("/api/v1/statements/generate", statementHandler)
	mux.Handle("/api/v1/exports/settlements.csv", apihttp.NewExportSettlementsCSVHandler(db, cfg.TenantID, stationChecker))
	mux.Handle("/api/v1/alarms/stream", alarmhttp.NewStreamHandler(alarmBroker))
	if alarmHandler, err := alarmhttp.NewHandler(alarmService, stationChecker); err == nil {
		mux.Handle("/api/v1/alarms", alarmHandler)
		mux.Handle("/api/v1/alarms/", alarmHandler)
	}
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: loggingMiddleware(authMiddleware.Wrap(mux), logger)}
	logger.Printf("http listening on %s", cfg.HTTPAddr)
	logger.Fatal(server.ListenAndServe())
}

type config struct {
	DatabaseURL             string
	HTTPAddr                string
	TenantID                string
	StationID               string
	PricePerKWh             float64
	Currency                string
	ExpectedHours           int
	TBBaseURL               string
	TBToken                 string
	AlarmWebhookURL         string
	AlarmNotifyTemplate     string
	AlarmEscalationAfter    time.Duration
	AlarmNotifyCooldown     time.Duration
	AlarmNotifyDedupeWindow time.Duration
	AlarmNotifyTimeout      time.Duration
	AlarmReportLookbackDays int
	AlarmReportBaseURL      string
	JWTSecret               string
	IngestSecret            string
	IngestSkewSeconds       int
}

func loadConfig() config {
	cfg := config{
		DatabaseURL:             getenvDefault("DATABASE_URL", getenvDefault("PG_DSN", "")),
		HTTPAddr:                getenvDefault("HTTP_ADDR", ":8080"),
		TenantID:                getenvDefault("TENANT_ID", "tenant-demo"),
		StationID:               getenvDefault("STATION_ID", "station-demo-001"),
		PricePerKWh:             getenvFloatDefault("PRICE_PER_KWH", 1.0),
		Currency:                getenvDefault("CURRENCY", "CNY"),
		ExpectedHours:           getenvIntDefault("EXPECTED_HOURS", 24),
		TBBaseURL:               getenvDefault("TB_BASE_URL", ""),
		TBToken:                 getenvDefault("TB_TOKEN", ""),
		AlarmWebhookURL:         getenvDefault("ALARM_WEBHOOK_URL", ""),
		AlarmNotifyTemplate:     getenvDefault("ALARM_NOTIFY_TEMPLATE", ""),
		AlarmEscalationAfter:    getenvDuration("ALARM_ESCALATION_AFTER", 0),
		AlarmNotifyCooldown:     getenvDuration("ALARM_NOTIFY_COOLDOWN", 0),
		AlarmNotifyDedupeWindow: getenvDuration("ALARM_NOTIFY_DEDUP_WINDOW", 0),
		AlarmNotifyTimeout:      getenvDuration("ALARM_NOTIFY_TIMEOUT", 5*time.Second),
		AlarmReportLookbackDays: getenvIntDefault("ALARM_REPORT_LOOKBACK_DAYS", 0),
		AlarmReportBaseURL:      getenvDefault("ALARM_REPORT_BASE_URL", getenvDefault("SHADOWRUN_PUBLIC_BASE_URL", "")),
		JWTSecret:               getenvDefault("AUTH_JWT_SECRET", getenvDefault("JWT_SECRET", "")),
		IngestSecret:            getenvDefault("INGEST_HMAC_SECRET", ""),
		IngestSkewSeconds:       getenvIntDefault("INGEST_MAX_SKEW_SECONDS", 300),
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL or PG_DSN is required")
	}
	if cfg.TBBaseURL == "" {
		log.Fatal("TB_BASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		log.Fatal("AUTH_JWT_SECRET is required")
	}
	return cfg
}

func getenvDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvFloatDefault(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvIntDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func buildShadowrunReportResolver(repo *shadowrepo.Repository, baseURL string, lookbackDays int) alarmnotify.ReportURLResolver {
	if repo == nil || baseURL == "" || lookbackDays <= 0 {
		return nil
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return func(ctx context.Context, alarm alarms.Alarm, _ *alarms.AlarmRule, _ *masterdata.Station) string {
		if alarm.StationID == "" {
			return ""
		}
		from := time.Now().UTC().AddDate(0, 0, -lookbackDays)
		to := time.Now().UTC().AddDate(0, 0, 1)
		reports, err := repo.ListReports(ctx, alarm.StationID, from, to)
		if err != nil || len(reports) == 0 {
			return ""
		}
		return baseURL + "/api/v1/shadowrun/reports/" + reports[0].ID + "/download"
	}
}

func loggingMiddleware(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		resp := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(resp, r)
		logger.Printf("http %s %s %d %s", r.Method, r.URL.Path, resp.status, time.Since(start))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// ---- Adapters ----

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type hourStatisticIDFactory struct{}

func (hourStatisticIDFactory) HourID(stationID string, hourStart time.Time) (domainstatistic.StatisticID, error) {
	_ = stationID
	return domainstatistic.BuildStatisticID(domainstatistic.GranularityHour, hourStart)
}

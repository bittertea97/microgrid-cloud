package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"microgrid-cloud/internal/analytics/application"
	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	appstatistic "microgrid-cloud/internal/analytics/application/statistic"
	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
	analyticsrepo "microgrid-cloud/internal/analytics/infrastructure/postgres"
	analyticsinterfaces "microgrid-cloud/internal/analytics/interfaces"
	apihttp "microgrid-cloud/internal/api/http"
	"microgrid-cloud/internal/eventing"
	eventingrepo "microgrid-cloud/internal/eventing/infrastructure/postgres"
	masterdatarepo "microgrid-cloud/internal/masterdata/infrastructure/postgres"
	settlementadapters "microgrid-cloud/internal/settlement/adapters/analytics"
	settlementapp "microgrid-cloud/internal/settlement/application"
	settlementrepo "microgrid-cloud/internal/settlement/infrastructure/postgres"
	settlementpricing "microgrid-cloud/internal/settlement/infrastructure/pricing"
	settlementinterfaces "microgrid-cloud/internal/settlement/interfaces"
	telemetryadapters "microgrid-cloud/internal/telemetry/adapters/analytics"
	telemetrypostgres "microgrid-cloud/internal/telemetry/infrastructure/postgres"
	thingsboard "microgrid-cloud/internal/telemetry/interfaces/thingsboard"

	_ "github.com/jackc/pgx/v5/stdlib"
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

	telemetryRepo := telemetrypostgres.NewTelemetryRepository(db)
	telemetryQuery := telemetrypostgres.NewTelemetryQuery(db)
	pointMappingRepo := masterdatarepo.NewPointMappingRepository(db)

	queryAdapter, err := telemetryadapters.NewQueryAdapter(cfg.TenantID, telemetryQuery, pointMappingRepo)
	if err != nil {
		logger.Fatalf("telemetry query adapter error: %v", err)
	}

	baseBus := eventbus.NewInMemoryBus()
	registry := eventing.NewRegistry()
	registry.Register(events.TelemetryWindowClosed{})
	registry.Register(events.StatisticCalculated{})
	registry.Register(settlementapp.SettlementCalculated{})

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

	ingestHandler, err := thingsboard.NewIngestHandler(telemetryRepo, logger)
	if err != nil {
		logger.Fatalf("ingest handler error: %v", err)
	}
	windowCloseHandler, err := analyticsinterfaces.NewWindowCloseHandler(publisher, logger)
	if err != nil {
		logger.Fatalf("window close handler error: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/ingest/thingsboard/telemetry", ingestHandler)
	mux.Handle("/analytics/window-close", windowCloseHandler)
	mux.Handle("/api/v1/stats", apihttp.NewStatsHandler(db))
	mux.Handle("/api/v1/settlements", apihttp.NewSettlementsHandler(db, cfg.TenantID))
	mux.Handle("/api/v1/exports/settlements.csv", apihttp.NewExportSettlementsCSVHandler(db, cfg.TenantID))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: loggingMiddleware(mux, logger)}
	logger.Printf("http listening on %s", cfg.HTTPAddr)
	logger.Fatal(server.ListenAndServe())
}

type config struct {
	DatabaseURL   string
	HTTPAddr      string
	TenantID      string
	StationID     string
	PricePerKWh   float64
	Currency      string
	ExpectedHours int
}

func loadConfig() config {
	cfg := config{
		DatabaseURL:   getenvDefault("DATABASE_URL", getenvDefault("PG_DSN", "")),
		HTTPAddr:      getenvDefault("HTTP_ADDR", ":8080"),
		TenantID:      getenvDefault("TENANT_ID", "tenant-demo"),
		StationID:     getenvDefault("STATION_ID", "station-demo-001"),
		PricePerKWh:   getenvFloatDefault("PRICE_PER_KWH", 1.0),
		Currency:      getenvDefault("CURRENCY", "CNY"),
		ExpectedHours: getenvIntDefault("EXPECTED_HOURS", 24),
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL or PG_DSN is required")
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

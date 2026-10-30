package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	// A imagem runtime é Alpine sem tzdata. Sem o embed, time.LoadLocation
	// falha para "America/Sao_Paulo" e o PUT /company-settings devolve 400.
	_ "time/tzdata"

	"github.com/oginoino/toca/pkg/tracing"

	"github.com/oginoino/toca-core/internal/addressverification"
	"github.com/oginoino/toca-core/internal/app"
	"github.com/oginoino/toca-core/internal/cache"
	"github.com/oginoino/toca-core/internal/handlers"
	repository "github.com/oginoino/toca-core/internal/repositories"
	service "github.com/oginoino/toca-core/internal/services"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/health"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/storage"
)

// @title           Toca Core API
// @version         1.0
// @description     Core service for Toca ecosystem (Properties, etc)
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8082
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {
	cfg := app.LoadAppConfig()

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-core")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}

	logger.Setup(cfg.Logger)
	logger.Info("Starting application", "app", "toca-core-api", "env", cfg.AppEnv)

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		logger.Error("Failed to connect to Postgres", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer pgDB.Close()
	logger.Info("Connected to Postgres database", "name", cfg.Database.Name)

	// Auditoria central: persiste os audit.Record do serviço (ex.: mutações
	// de propriedade e de settings) e as negações de autorização do pkg/auth
	// em audit.events via pool de serviço.
	audit.SetDefault(audit.NewRecorder(pgDB))

	storageService, err := storage.NewFromEnv()
	if err != nil {
		logger.Error("Failed to initialize storage — aborting startup", "error", err)
		os.Exit(1)
	}

	// Services
	logger.Info("Using Postgres Repository for Properties", nil)
	propertyRepo := repository.NewPropertyPostgresRepository(pgDB)
	// crossRepo lets PropertyService validate broker/owner-lead IDs against
	// the crm.* schemas during the captação flow (Phase 1, item 5).
	crossRepo := repository.NewCrossPostgresRepository(pgDB)
	propertyService := service.NewPropertyServiceWithCrossRepo(propertyRepo, crossRepo)
	verificationMode, err := addressverification.ParseMode(cfg.AddressVerification.Mode)
	if err != nil {
		logger.Error("Invalid address verification configuration", "error", err)
		os.Exit(1)
	}
	var addressProvider addressverification.Provider
	if verificationMode != addressverification.ModeDisabled {
		if !strings.EqualFold(strings.TrimSpace(cfg.AddressVerification.Provider), "viacep") {
			logger.Error("Unsupported address verification provider", "provider", cfg.AddressVerification.Provider)
			os.Exit(1)
		}
		addressProvider, err = addressverification.NewViaCEPProvider(
			cfg.AddressVerification.BaseURL,
			&http.Client{Timeout: cfg.AddressVerification.Timeout},
		)
		if err != nil {
			logger.Error("Failed to initialize address verification provider", "error", err)
			os.Exit(1)
		}
	}
	addressProcessor, err := addressverification.NewProcessor(addressProvider, addressverification.ProcessorConfig{
		Mode:            verificationMode,
		Timeout:         cfg.AddressVerification.Timeout,
		CacheTTL:        cfg.AddressVerification.CacheTTL,
		CacheMaxEntries: cfg.AddressVerification.CacheMaxEntries,
		OnFallback: func(ctx context.Context, err error) {
			logger.WarnContext(ctx, "Address verification degraded to normalized input", "provider", cfg.AddressVerification.Provider, "error", err)
		},
	})
	if err != nil {
		logger.Error("Failed to initialize address verification", "error", err)
		os.Exit(1)
	}
	propertyService.SetAddressProcessor(addressProcessor)
	logger.Info("Address verification configured", "mode", verificationMode, "provider", cfg.AddressVerification.Provider)

	partnershipRepo := repository.NewPartnershipPostgresRepository(pgDB)
	partnershipService := service.NewPartnershipService(partnershipRepo)
	// The shared-portfolio endpoint needs the property repo to list the
	// counterpart company's properties (cross-tenant, gated by the
	// partnership permission check inside the service).
	partnershipService.SetPropertyRepository(propertyRepo)

	companyRepo := repository.NewCompanySettingsPostgresRepository(pgDB)
	appearanceRepo := repository.NewAppearancePostgresRepository(pgDB)
	permissionsRepo := repository.NewPermissionsPostgresRepository(pgDB)
	notificationsRepo := repository.NewNotificationSettingsPostgresRepository(pgDB)
	webhookRepo := repository.NewWebhookPostgresRepository(pgDB)
	apiKeyRepo := repository.NewAPIKeyPostgresRepository(pgDB)

	settingsService := service.NewSettingsService(companyRepo, appearanceRepo, permissionsRepo, notificationsRepo, webhookRepo, apiKeyRepo, storageService)
	// A5: invalidação do cache de permissões ao salvar a matriz (opcional;
	// sem REDIS_URI o serviço opera normalmente, com propagação por TTL).
	if inv := cache.FromEnv(); inv != nil {
		settingsService.WithPermissionsInvalidator(inv)
	}

	// RabbitMQ & Events
	var rb *events.RabbitMQEventBus
	if cfg.RabbitMQ.URI != "" {
		rb, err = events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
		if err != nil {
			logger.Error("Failed to connect to RabbitMQ", "error", err)
		} else {
			defer rb.Close()
			logger.Info("Connected to RabbitMQ")

			// Register Handler
			dealHandler := service.NewDealEventHandler(propertyService)
			if err := rb.SubscribeWithQueue("toca-core.deal.status_changed", "deal.status_changed", dealHandler); err != nil {
				logger.Error("Failed to subscribe to deal events", "error", err)
			} else {
				logger.Info("Subscribed to deal.status_changed")
			}

			rb.StartDLQConsumers()
		}
	} else {
		logger.Warn("RabbitMQ URI not configured. Event subscription disabled.")
	}

	// rb may be nil here if RabbitMQ wasn't configured — SetupRouter handles
	// the nil case by passing a nil event bus to the captação aggregator
	// (which then degrades to "persist without emitting").
	var eventBusForRoutes events.EventBus
	if rb != nil {
		eventBusForRoutes = rb
		// Wire the event bus into PropertyService so Update() emits
		// property.updated for downstream services that snapshot property
		// data (e.g. toca-deal Contract.PropertyTitle).
		propertyService.SetEventBus(rb)
		// Wire the event bus into PartnershipService so it emits
		// partnership.invited / partnership.accepted for toca-comm to
		// notify the involved companies.
		partnershipService.SetEventBus(rb)
	}
	router := handlers.SetupRouter(cfg, propertyService, partnershipService, settingsService, storageService, eventBusForRoutes)
	// RequestID, Logger, RequestTimeout and Recovery are now part of
	// the base stack applied inside SetupRouter via pkgmw.Setup.
	// Tracing stays here because it needs the service name.
	router.Use(tracing.Middleware("toca-core"))
	if rb != nil {
		events.RegisterDLQAdminRoutes(router, rb)
	}
	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(router, "toca-core", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(router, "toca-core", deep)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("Server started", "port", cfg.Port)

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server exiting")
}

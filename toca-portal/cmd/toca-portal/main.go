package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oginoino/toca/pkg/tracing"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/health"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/toca-portal/internal/app"
	"github.com/oginoino/toca/toca-portal/internal/handlers"
	repository "github.com/oginoino/toca/toca-portal/internal/repositories"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

// @title           Toca Portal API
// @version         1.0
// @description     Portal integration service for Toca ecosystem - ZAP Imoveis, OLX/Grupo ZAP, VivaReal

// @contact.name   API Support
// @contact.email  support@toca.com.br

// @host      localhost:8089
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-portal")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}
	// Load configuration
	cfg := config.LoadBaseConfig()

	// Service-specific configuration
	coreServiceURL := os.Getenv("CORE_SERVICE_URL")
	if coreServiceURL == "" {
		coreServiceURL = "http://localhost:8082"
	}

	crmServiceURL := os.Getenv("CRM_SERVICE_URL")
	if crmServiceURL == "" {
		crmServiceURL = "http://localhost:8084"
	}

	feedBaseURL := os.Getenv("FEED_BASE_URL")
	if feedBaseURL == "" {
		feedBaseURL = "http://localhost:8080"
	}

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pgDB.Close()
	log.Printf("Connected to Postgres database: %s", cfg.Database.Name)

	// Auditoria central: persiste as negações de autorização do pkg/auth e
	// os audit.Record do serviço (integrações/sincronizações) em audit.events.
	audit.SetDefault(audit.NewRecorder(pgDB))

	// Setup EventBus
	var eventBus events.EventBus
	var rb *events.RabbitMQEventBus
	if cfg.RabbitMQ.URI != "" {
		rb, err = events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
		if err != nil {
			log.Printf("Failed to connect to RabbitMQ: %v. Using InMemory fallback.", err)
			eventBus = events.NewInMemoryEventBus()
		} else {
			log.Println("Connected to RabbitMQ")
			defer rb.Close()
			eventBus = rb
		}
	} else {
		log.Println("RabbitMQ URI not configured. Using InMemory EventBus.")
		eventBus = events.NewInMemoryEventBus()
	}

	// Repositories
	integRepo := repository.NewIntegrationPostgresRepository(pgDB)
	syncRepo := repository.NewSyncPostgresRepository(pgDB)
	leadRepo := repository.NewPortalLeadPostgresRepository(pgDB)
	cacheRepo := repository.NewFeedCachePostgresRepository(pgDB)

	// Services
	integService := service.NewIntegrationService(integRepo, feedBaseURL)
	feedService := service.NewFeedService(cacheRepo, coreServiceURL, cfg.InternalAPIKey)
	webhookService := service.NewWebhookService(leadRepo, integRepo, eventBus, crmServiceURL, cfg.InternalAPIKey)
	syncService := service.NewSyncService(syncRepo, integRepo, feedService, eventBus)

	// Handlers
	integHandler := handlers.NewIntegrationHandler(integService)
	feedHandler := handlers.NewFeedHandler(integService, feedService)
	webhookHandler := handlers.NewWebhookHandler(webhookService)
	syncHandler := handlers.NewSyncHandler(syncService)
	statsHandler := handlers.NewStatsHandler(integService, leadRepo)

	// Setup Router
	r := handlers.SetupRouter(cfg, integHandler, feedHandler, webhookHandler, syncHandler, statsHandler)
	r.Use(pkgmw.RequestID())
	r.Use(tracing.Middleware("toca-portal"))
	r.Use(pkgmw.RequestTimeout(0))
	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(r, "toca-portal", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(r, "toca-portal", deep)

	// Start Scheduler for periodic syncs
	scheduler := app.NewScheduler(integRepo, syncService)
	scheduler.Start()
	defer scheduler.Stop()

	// Start Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Starting Portal service on port %s", cfg.Port)
		log.Printf("Core Service: %s", coreServiceURL)
		log.Printf("CRM Service: %s", crmServiceURL)
		log.Printf("Feed Base URL: %s", feedBaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

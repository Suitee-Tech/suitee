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
	"github.com/oginoino/toca/pkg/pdf"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/toca-ops/internal/handlers"
	"github.com/oginoino/toca/toca-ops/internal/repositories/postgres"
	"github.com/oginoino/toca/toca-ops/internal/services"
)

func main() {

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-ops")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}
	// Load configuration
	cfg := config.LoadBaseConfig()

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pgDB.Close()
	log.Printf("Connected to Postgres database: %s", cfg.Database.Name)

	// Wire global audit recorder. Once SetDefault is called, audit.Record()
	// no service layer começa a persistir em audit.events. Antes disso,
	// chamadas só produzem log estruturado (seguro para start-up).
	audit.SetDefault(audit.NewRecorder(pgDB))

	// Repositories
	maintenanceRepo := postgres.NewMaintenanceRepository(pgDB)
	keyRepo := postgres.NewKeyRepository(pgDB)
	inspectionRepo := postgres.NewInspectionRepository(pgDB)
	ticketRepo := postgres.NewTicketRepository(pgDB)

	// Optional EventBus
	var eventBus events.EventBus
	var rb *events.RabbitMQEventBus
	if cfg.RabbitMQ.URI != "" {
		rb, err = events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
		if err != nil {
			log.Printf("Failed to connect to RabbitMQ: %v", err)
		} else {
			defer rb.Close()
			eventBus = rb
			log.Println("Connected to RabbitMQ")
		}
	}

	// Services
	maintenanceService := services.NewMaintenanceService(maintenanceRepo, eventBus)
	keyService := services.NewKeyService(keyRepo)
	supplierRepo := postgres.NewSupplierRepository(pgDB)
	supplierService := services.NewSupplierService(supplierRepo)
	inspectionService := services.NewInspectionService(inspectionRepo, eventBus).WithSuppliers(supplierRepo)
	ticketService := services.NewTicketService(ticketRepo, eventBus)
	// Issue #440 — wire storage for inspection photo uploads. Quando o
	// ambiente não configurou storage (dev local), o serviço continua
	// funcionando para tudo exceto upload, que devolve 500 com mensagem
	// clara ("storage service not configured").
	if storageService, err := storage.NewFromEnv(); err != nil {
		log.Printf("WARN: storage not initialized: %v (uploads will fail with 500)", err)
	} else {
		inspectionService.WithStorage(storageService)
		maintenanceService.WithStorage(storageService)
		ticketService.WithStorage(storageService)
		log.Println("Storage configured for inspection photo uploads")
	}
	// Issue #435 — wire renderer de PDF para gerar laudo de vistoria.
	// Sem GOTENBERG_URL, NewRendererFromEnv devolve NoopRenderer e
	// GenerateLaudoPDF falha com erro descritivo (503) — operador
	// continua podendo anexar laudo manual.
	pdfRenderer := pdf.NewRendererFromEnv()
	inspectionService.WithPDFRenderer(pdfRenderer)
	if pdfRenderer.Available() {
		log.Println("PDF renderer configured (laudo generation enabled)")
	} else {
		log.Println("GOTENBERG_URL not set; laudo PDF generation disabled (manual ReportURL still works)")
	}
	// FASE 2 (integração total) — react to deal.contract.signed by
	// scheduling an ENTRY inspection on the contract's StartDate. The
	// inspection id is derived from the contract id so broker reentries
	// no-op via CreateScheduled.
	if rb != nil {
		entryInspectionHandler := services.NewContractSignedInspectionHandler(inspectionService)
		if err := rb.SubscribeWithQueue("toca-ops.deal.contract.signed", "deal.contract.signed", entryInspectionHandler); err != nil {
			log.Printf("Failed to subscribe to deal.contract.signed events: %v", err)
		} else {
			log.Println("Subscribed to deal.contract.signed (toca-ops)")
		}
	}

	// Setup Router
	r := handlers.SetupRouter(cfg, maintenanceService, keyService, inspectionService, ticketService, supplierService)
	r.Use(pkgmw.RequestID())
	r.Use(tracing.Middleware("toca-ops"))
	r.Use(pkgmw.RequestTimeout(0))
	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(r, "toca-ops", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(r, "toca-ops", deep)

	// Start Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Starting Ops service on port %s", cfg.Port)
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

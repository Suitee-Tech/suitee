package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/oginoino/toca/pkg/tracing"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/gdpr"
	"github.com/oginoino/toca/pkg/health"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-crm/internal/api/v1/routes"
	"github.com/oginoino/toca/toca-crm/internal/service"
)

// @title           Toca CRM API
// @version         1.0
// @description     CRM service for Toca ecosystem
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8083
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-crm")
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

	// Audit + LGPD wiring. The recorder writes to audit.events via the
	// service pool; the LeadAnonymizer registers as a gdpr.Anonymizer so
	// the orchestrator (toca-auth) reaches it on a deletion request.
	audit.SetDefault(audit.NewRecorder(pgDB))
	gdpr.Register(service.NewLeadAnonymizer(pgDB))
	log.Printf("Connected to Postgres database: %s", cfg.Database.Name)

	// Issue #427: SLA Primeiro Contato. Default 60min (1h), override
	// via LEAD_FIRST_CONTACT_SLA_MINUTES. Quando inválido (não-numérico
	// ou <= 0), mantém o default — fail-safe pra não deixar SLA
	// efetivamente desligado por typo de config.
	if v := os.Getenv("LEAD_FIRST_CONTACT_SLA_MINUTES"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			service.FirstContactSLAMinutes = n
			log.Printf("LEAD_FIRST_CONTACT_SLA_MINUTES overridden to %d", n)
		} else {
			log.Printf("LEAD_FIRST_CONTACT_SLA_MINUTES invalid (%q), keeping default %d", v, service.FirstContactSLAMinutes)
		}
	}

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

	// workerCtx é cancelado no graceful shutdown abaixo para encerrar o
	// SLA worker (e qualquer worker futuro) sem vazar goroutine.
	workerCtx, cancelWorkers := context.WithCancel(context.Background())

	// Setup Storage (necessário para export jobs CSV/PDF — #474).
	// Sem storage configurado, exports continuam quase funcionando
	// (job é criado em QUEUED) mas worker falha no upload e marca
	// FAILED. Log explícito para o operador notar.
	storageService, err := storage.NewFromEnv()
	if err != nil {
		log.Printf("Warning: storage init failed (%v) — exports will fail at upload step", err)
	}

	// Setup Router
	r := routes.SetupRouter(workerCtx, cfg, pgDB, eventBus, storageService)
	// RequestID, Logger, RequestTimeout and Recovery are now part of
	// the base stack applied inside SetupRouter via pkgmw.Setup.
	// Tracing stays here because it needs the service name.
	r.Use(tracing.Middleware("toca-crm"))
	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(r, "toca-crm", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(r, "toca-crm", deep)

	// Start Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Starting CRM service on port %s", cfg.Port)

		// Fuso configurado pela imobiliária (Configurações → "Fuso horário") manda
		// nos dias civis dos fluxos; sem configuração/core, cai no padrão do
		// ambiente (America/Sao_Paulo).
		timeutil.SetTenantResolver(clients.NewCoreClient().TenantLocationResolver())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
	cancelWorkers() // sinaliza o SLA worker (e demais background jobs) para parar

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

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
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/health"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-calendar/internal/handlers"
	"github.com/oginoino/toca/toca-calendar/internal/repositories"
	"github.com/oginoino/toca/toca-calendar/internal/services"
)

func main() {

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-calendar")
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

	// Auditoria central: persiste os audit.Record do serviço (agendamentos)
	// e as negações de autorização do pkg/auth em audit.events.
	audit.SetDefault(audit.NewRecorder(pgDB))

	// Setup repositories
	apptRepo := repositories.NewAppointmentPostgresRepository(pgDB)

	// Setup optional event bus
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

	// Setup services
	//
	// Link assinado do .ics no e-mail: usa CALENDAR_LINK_SECRET quando existe e
	// cai para a chave interna (já presente em todos os ambientes) para o CTA
	// funcionar sem exigir configuração nova. Sem URL pública o link fica
	// inerte e o e-mail sai sem o botão.
	calendarSecret := os.Getenv("CALENDAR_LINK_SECRET")
	if calendarSecret == "" {
		calendarSecret = os.Getenv("INTERNAL_API_KEY")
	}
	apptService := services.NewAppointmentService(apptRepo, eventBus).
		WithCalendarLinks(services.NewCalendarLink(calendarSecret, os.Getenv("PUBLIC_API_URL")))

	if rb != nil {
		if err := rb.SubscribeWithQueue("toca-calendar.inspection.calendar_changed", "inspection.calendar_changed", services.NewInspectionCalendarHandler(apptRepo)); err != nil {
			log.Fatalf("Failed to subscribe inspection calendar sync: %v", err)
		}
	}

	// Setup Router
	r := handlers.SetupRouter(cfg, apptService)
	r.Use(pkgmw.RequestID())
	r.Use(tracing.Middleware("toca-calendar"))
	r.Use(pkgmw.RequestTimeout(0))
	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(r, "toca-calendar", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(r, "toca-calendar", deep)

	// Start Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Starting calendar service on port %s", cfg.Port)

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

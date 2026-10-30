package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oginoino/toca/pkg/tracing"

	"github.com/oginoino/toca-auth/internal/api/v1/routes"
	"github.com/oginoino/toca-auth/internal/app"
	redisclient "github.com/oginoino/toca-auth/internal/infrastructure/redis"
	"github.com/oginoino/toca-auth/internal/repository"
	"github.com/oginoino/toca-auth/internal/service"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/gdpr"
	"github.com/oginoino/toca/pkg/health"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/storage"
)

// @title           Toca Auth API
// @version         1.0
// @description     Authentication service for Toca ecosystem
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {
	// Load configuration
	cfg := app.LoadAppConfig()

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-auth")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		logger.Error("Failed to initialize OpenTelemetry", "error", err)
	}

	// Setup logger
	logger.Setup(cfg.Logger)
	log := logger.Get()

	log.Info("Starting application", "app", "toca-auth-service", "env", cfg.AppEnv)

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		log.Error("Failed to connect to Postgres", "error", err)
		os.Exit(1)
	}
	defer pgDB.Close()
	log.Info("Connected to Postgres database", "db", cfg.Database.Name)

	// Wire global audit recorder. Logs de auth (login_success/failed,
	// switch_context, banking updates) ficam visíveis no /audit-trail
	// global da J10 BDD. Sem isso, eventos de auth não aparecem nas
	// investigações de incidente (ex.: "5 logins falhos em 1 min" da
	// variação de brute force).
	audit.SetDefault(audit.NewRecorder(pgDB))

	// LGPD anonymizer (#476). Registra o UserAnonymizer no registry
	// global do pkg/gdpr — quando DELETE /lgpd/me/account dispara
	// gdpr.AnonymizeAccount, o auth.account do usuário é anonimizado
	// junto com leads/deals etc. (cada service registra o seu).
	gdpr.Register(service.NewUserAnonymizer(pgDB))

	if err := repository.BootstrapAuthSchema(context.Background(), pgDB); err != nil {
		log.Error("Failed to bootstrap auth schema", "error", err)
		os.Exit(1)
	}

	// Setup router
	storageService, err := storage.NewFromEnv()
	if err != nil {
		logger.Error("Failed to initialize storage — aborting startup", "error", err)
		os.Exit(1)
	}

	var bus events.EventBus
	rmqBus, err := events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
	if err != nil {
		logger.Warn("Failed to connect to RabbitMQ", "error", err)
	} else {
		bus = rmqBus
		defer rmqBus.Close()
	}

	r := routes.SetupRouter(cfg, pgDB, storageService, bus)
	// RequestID, Logger, RequestTimeout and Recovery are now part of
	// the base stack applied inside SetupRouter via pkgmw.Setup.
	// Tracing stays here because it needs the service name.
	r.Use(tracing.Middleware("toca-auth"))
	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rmqBus != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rmqBus))
	}
	if healthRedis, err := redisclient.New(cfg.Redis.URI); err == nil {
		readiness.Add(health.NewRedisChecker(healthRedis.Raw()))
	}
	health.RegisterGin(r, "toca-auth", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rmqBus != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rmqBus))
	}
	if deepRedis, err := redisclient.New(cfg.Redis.URI); err == nil {
		deep.Add(health.NewRedisDeepChecker(deepRedis.Raw(), "health:probe:auth"))
	}
	health.RegisterDeepGin(r, "toca-auth", deep)

	// Create server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start server in a goroutine
	go func() {
		log.Info("Server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	log.Info("Server exiting")
}

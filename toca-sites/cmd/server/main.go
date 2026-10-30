package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/health"
	"github.com/oginoino/toca/pkg/tracing"

	"github.com/oginoino/toca/toca-sites/internal/api/handlers"
	"github.com/oginoino/toca/toca-sites/internal/api/middleware"
	"github.com/oginoino/toca/toca-sites/internal/api/routes"
	"github.com/oginoino/toca/toca-sites/internal/client/core"
	"github.com/oginoino/toca/toca-sites/internal/client/crm"
	"github.com/oginoino/toca/toca-sites/internal/client/payload"
	"github.com/oginoino/toca/toca-sites/internal/domain/site"
	"github.com/oginoino/toca/toca-sites/internal/repository/postgres"
	"github.com/oginoino/toca/toca-sites/internal/services"
	"github.com/oginoino/toca/toca-sites/internal/tlsconfig"
)

// @title           Toca Sites API
// @version         1.0
// @description     Vitrine pública multi-tenant das imobiliárias (ADR 0006). Site público resolvido pelo Host comercial e APIs do painel.
// @host            localhost:8094
// @BasePath        /api/v1

func main() {
	shutdown, err := tracing.InitProvider("toca-sites")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}

	cfg := config.LoadBaseConfig()
	if strings.TrimSpace(os.Getenv("PORT")) == "" {
		cfg.Port = "8094"
	}

	hostSuffix := strings.Trim(strings.TrimSpace(config.GetEnv("PUBLIC_SITE_HOST_SUFFIX", "imoveis.suitee.com.br")), ".")
	scheme := strings.TrimSpace(config.GetEnv("SITE_PUBLIC_SCHEME", "https"))
	publicPort := strings.TrimSpace(os.Getenv("SITE_PUBLIC_PORT"))
	payloadURL := strings.TrimSpace(os.Getenv("PAYLOAD_API_URL"))
	coreURL := strings.TrimSpace(os.Getenv("CORE_SERVICE_URL"))
	if coreURL == "" {
		coreURL = "http://localhost:8082"
	}
	crmURL := strings.TrimSpace(os.Getenv("CRM_SERVICE_URL"))
	if crmURL == "" {
		crmURL = "http://localhost:8084"
	}
	publicRate := parseFloatEnv("SITES_PUBLIC_RATE_LIMIT_PER_MIN", 120)
	publicOrigin := strings.TrimSpace(os.Getenv("SITES_PUBLIC_CORS_ORIGIN"))

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pgDB.Close()

	audit.SetDefault(audit.NewRecorder(pgDB))

	// Barramento: sem RabbitMQ o serviço continua atendendo (projeção local +
	// endpoints públicos), mas perde o sync automático — por isso o boot é
	// barulhento.
	var bus events.EventBus
	var rabbit *events.RabbitMQEventBus
	if cfg.RabbitMQ.URI != "" {
		rabbit, err = events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
		if err != nil {
			log.Printf("Failed to connect to RabbitMQ: %v. Sync automático desabilitado.", err)
		} else {
			defer rabbit.Close()
			bus = rabbit
		}
	}
	if bus == nil {
		log.Println("RabbitMQ indisponível: toca-sites sobe sem consumir property.updated")
	}

	repo := postgres.New(pgDB)

	tlsCtx, tlsCancel := context.WithCancel(context.Background())
	defer tlsCancel()
	if tlsDir := strings.TrimSpace(os.Getenv("TRAEFIK_DYNAMIC_DIR")); tlsDir != "" {
		upstream := strings.TrimSpace(os.Getenv("TRAEFIK_SITE_UPSTREAM"))
		if upstream == "" {
			upstream = "http://toca-web:3000"
		}
		go tlsconfig.NewWriter(repo, tlsDir, hostSuffix, upstream).Run(tlsCtx)
		log.Printf("TLS dinâmico da vitrine ativo em %s", tlsDir)
	}

	var projection site.Projection = payload.Noop{}
	if payloadURL != "" {
		projection = payload.New(payloadURL, cfg.InternalAPIKey, hostSuffix)
	} else {
		log.Println("PAYLOAD_API_URL não configurado: projeção no CMS desabilitada")
	}

	// Só publica se houver barramento. Passar um adapter nulo (interface com
	// ponteiro nil) faria o serviço achar que publica e panicar na primeira
	// chamada.
	var reliable services.ReliablePublisher
	if bus != nil {
		reliable = publisherAdapter{bus: bus}
	}

	siteService := services.NewSiteService(repo, repo, repo, projection, hostSuffix, scheme).
		WithPublicPort(publicPort)
	syncService := services.NewSyncService(repo, repo, core.New(coreURL, cfg.InternalAPIKey), projection,
		reliable, hostSuffix, scheme).WithPublicPort(publicPort)
	engagementService := services.NewEngagementService(repo, repo, repo,
		crm.New(crmURL, cfg.InternalAPIKey), reliable, hostSuffix)

	// Primeiro enable da vitrine faz backfill do estoque existente (o evento
	// só cobre mudanças futuras).
	siteService.WithReconciler(syncService)

	if bus != nil {
		consumer := services.NewPropertyConsumer(bus, syncService)
		if err := consumer.Start(); err != nil {
			log.Printf("Falha ao assinar eventos de imóvel: %v", err)
		} else {
			log.Printf("Consumindo eventos nas filas %s e %s",
				services.QueuePropertyUpdated, services.QueuePropertyCaptured)
		}
	}

	siteHandler := handlers.NewSiteHandler(siteService, syncService)
	publicHandler := handlers.NewPublicHandler(engagementService)
	guard := middleware.NewPublicGuard(scheme, hostSuffix, publicOrigin, publicRate)

	publicOriginPattern := scheme + "://*." + hostSuffix
	r := routes.SetupRouter(routes.Deps{
		Cfg:                 cfg,
		SiteHandler:         siteHandler,
		PublicHandler:       publicHandler,
		PublicGuard:         guard,
		PublicOriginPattern: publicOriginPattern,
	})

	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rabbit != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rabbit))
	}
	health.RegisterGin(r, "toca-sites", readiness)
	health.RegisterDeepGin(r, "toca-sites", health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB)))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Starting toca-sites on port %s (host suffix: %s, payload: %s)", cfg.Port, hostSuffix, payloadURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down toca-sites...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
}

func parseFloatEnv(key string, def float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// publisherAdapter expõe só PublishReliable para os serviços. EventBus é uma
// interface maior; mantê-la local evita que domínio/serviço dependam de
// Subscribe/Publish quando só publicam.
type publisherAdapter struct {
	bus events.EventBus
}

func (p publisherAdapter) PublishReliable(ctx context.Context, event events.Event) error {
	if p.bus == nil {
		return nil
	}
	return p.bus.PublishReliable(ctx, event)
}

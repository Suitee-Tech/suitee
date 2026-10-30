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

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/health"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/tracing"
	"github.com/oginoino/toca/toca-signature/internal/api/v1/routes"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
	"github.com/oginoino/toca/toca-signature/internal/pdf"
	"github.com/oginoino/toca/toca-signature/internal/repository"
	"github.com/oginoino/toca/toca-signature/internal/service"
)

// @title           Toca Signature API
// @version         1.0
// @description     Signature service (Clicksign integration) for the Toca ecosystem
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.email  support@suitee.com.br

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8093
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {
	shutdown, err := tracing.InitProvider("toca-signature")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}

	cfg := config.LoadBaseConfig()

	// Comando operacional, antes de qualquer dependencia: registrar webhook só
	// precisa das credenciais do provedor — não do banco nem do resto do serviço.
	if len(os.Args) > 1 && os.Args[1] == "register-webhook" {
		os.Exit(runRegisterWebhook(clicksign.NewClient(credential.NewEnvCredentialProvider())))
	}

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pgDB.Close()
	log.Printf("Connected to Postgres database: %s", cfg.Database.Name)

	// Auditoria central: persiste os audit.Record do ciclo de vida dos
	// envelopes (criação, status do provedor, reconciliação) e as negações
	// de autorização do pkg/auth em audit.events.
	audit.SetDefault(audit.NewRecorder(pgDB))

	// Event bus — required for publishing signature.* events back to the rest
	// of the platform. We tolerate misconfigured RabbitMQ for local dev but
	// log loudly because none of the downstream pipeline (toca-deal status
	// updates) will work without it.
	var rb *events.RabbitMQEventBus
	var eventBus events.EventBus
	if cfg.RabbitMQ.URI != "" {
		rb, err = events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
		if err != nil {
			log.Printf("Failed to connect to RabbitMQ: %v. Falling back to InMemory bus.", err)
			eventBus = events.NewInMemoryEventBus()
		} else {
			log.Println("Connected to RabbitMQ")
			defer rb.Close()
			eventBus = rb
		}
	} else {
		log.Println("RabbitMQ URI not configured — using InMemory bus (events go nowhere outside this process)")
		eventBus = events.NewInMemoryEventBus()
	}

	// Wire dependencies for the signature pipeline.
	// P1-14: credenciais por tenant. A linha do tenant (quando existe)
	// vence o fallback global de ambiente; a chave de criptografia em
	// repouso vem de CLICKSIGN_CREDENTIALS_ENC_KEY.
	envProvider := credential.NewEnvCredentialProvider()
	credsProvider := credential.CredentialProvider(envProvider)
	var credsAdmin *credential.DBCredentialProvider
	if encRaw := strings.TrimSpace(os.Getenv("CLICKSIGN_CREDENTIALS_ENC_KEY")); encRaw != "" {
		encKey, err := credential.KeyFromHex(encRaw)
		if err != nil {
			log.Printf("[WARN] P1-14: chave de credenciais por tenant inválida — usando apenas credenciais globais: %v", err)
		} else {
			credRepo := repository.NewClicksignCredentialsPostgresRepository(pgDB)
			dbProvider := credential.NewDBCredentialProvider(credRepo, encKey, envProvider)
			credsProvider = dbProvider
			credsAdmin = dbProvider
		}
	} else {
		log.Println("[INFO] P1-14: CLICKSIGN_CREDENTIALS_ENC_KEY ausente — credenciais por tenant desligadas (fallback global)")
	}
	clicksignClient := clicksign.NewClient(credsProvider)
	dealClient := clients.NewDealClient()
	envelopeRepo := repository.NewEnvelopePostgresRepository(pgDB)

	// Pick the PDF rendering strategy from env. With GOTENBERG_URL set, HTML
	// contracts are rendered to PDF before being uploaded to Clicksign.
	// WithDB enables the atomic publish+commit path for both services:
	// webhook handlers (WebhookService) and the operator Reconcile path
	// (EnvelopeService) now write the envelope row and the matching
	// outbox events in a single Postgres transaction. See audit P0.1
	// follow-up + signature/internal/service/{webhook,envelope}_service.go.
	envelopeService := service.NewEnvelopeService(envelopeRepo, clicksignClient, credsProvider, dealClient, pdf.NewFromEnv(), eventBus).WithDB(pgDB)
	webhookService := service.NewWebhookService(envelopeRepo, eventBus).WithDB(pgDB).WithClicksign(clicksignClient)

	// Subscribe to signature.requested. Only do this if RabbitMQ is up —
	// the InMemory bus would never deliver these because the publisher
	// (toca-deal) is in a different process.
	if rb != nil {
		dealSub := service.NewDealSubscriber(envelopeService)
		if err := rb.SubscribeWithQueue("toca-signature.signature.requested", "signature.requested", dealSub); err != nil {
			log.Printf("Failed to subscribe to signature.requested: %v", err)
		}
		rb.StartDLQConsumers()
	}

	// Rede de segurança: webhook pode atrasar ou se perder, e sem isso o
	// envelope fica preso em RUNNING do nosso lado mesmo já finalizado no
	// provedor — deixando o contrato em PENDING_SIGNATURE sem ninguém perceber.
	sweeperCtx, stopSweeper := context.WithCancel(context.Background())
	defer stopSweeper()
	sweeper := service.NewReconcileSweeper(envelopeService, envelopeRepo, 0)
	go sweeper.Start(sweeperCtx)

	r := routes.SetupRouter(cfg, pgDB, routes.Deps{
		Repo:            envelopeRepo,
		Creds:           credsProvider,
		WebhookService:  webhookService,
		EnvelopeService: envelopeService,
		CredentialAdmin: credsAdmin,
	})
	r.Use(pkgmw.RequestID())
	r.Use(tracing.Middleware("toca-signature"))
	r.Use(pkgmw.RequestTimeout(0))

	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(r, "toca-signature", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(r, "toca-signature", deep)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Surface the public webhook URL so the operator knows what to paste
	// into the Clicksign dashboard. Derived from PUBLIC_GATEWAY_URL (or the
	// WEBHOOK_PUBLIC_URL override). Empty means PUBLIC_GATEWAY_URL is unset
	// — webhook delivery from Clicksign won't reach us.
	if url := clicksign.PublicWebhookURL(); url != "" {
		log.Printf("Clicksign webhook URL (register this in your Clicksign dashboard): %s", url)
	} else {
		log.Println("WARNING: PUBLIC_GATEWAY_URL is not set — Clicksign won't be able to reach the webhook. Set it in .env (e.g. https://abc.ngrok-free.app for local dev) and restart.")
	}

	go func() {
		log.Printf("Starting Signature service on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
	// Encerra a varredura antes do servidor para ela não falar com o provedor
	// durante o desligamento.
	stopSweeper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

// runRegisterWebhook cadastra (ou confirma) a URL de webhook na Clicksign.
//
// A URL vem de WEBHOOK_PUBLIC_URL ou PUBLIC_GATEWAY_URL, com o caminho
// canônico anexado — a mesma resolução que o serviço usa para exibir a URL ao
// operador, então o que é registrado é exatamente o que o serviço espera.
func runRegisterWebhook(client *clicksign.Client) int {
	url := clicksign.PublicWebhookURL()
	if url == "" {
		log.Println("WEBHOOK_PUBLIC_URL e PUBLIC_GATEWAY_URL estão vazias: não há URL para registrar.")
		log.Println("Defina PUBLIC_GATEWAY_URL (ex.: https://app.example.com) e rode de novo.")
		return 1
	}

	// O provedor por env não separa tenants; o identificador só rotula o cache
	// interno do cliente.
	tenantID := strings.TrimSpace(os.Getenv("CLICKSIGN_WEBHOOK_TENANT_ID"))
	if tenantID == "" {
		tenantID = "default"
	}

	log.Printf("registrando webhook na Clicksign: %s", url)
	created, existing, err := client.EnsureWebhook(context.Background(), tenantID, url)
	if err != nil {
		log.Printf("falha ao registrar: %v", err)
		log.Println("A Clicksign valida que a URL responde antes de aceitar o cadastro —")
		log.Println("confirme que o endpoint está publicado e responde POST em " + url)
		return 1
	}
	if !created {
		log.Printf("webhook já cadastrado (id=%s); nada a fazer", existing.ID)
		return 0
	}
	log.Printf("webhook cadastrado: id=%s url=%s", existing.ID, existing.Attributes.URL)
	return 0
}

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
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/toca-comm/internal/api/v1/handlers"
	"github.com/oginoino/toca/toca-comm/internal/api/v1/routes"
	"github.com/oginoino/toca/toca-comm/internal/repository"
	"github.com/oginoino/toca/toca-comm/internal/service"
)

// @title           Toca Comm API
// @version         1.0
// @description     Communication service for Toca ecosystem
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8081
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-comm")
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

	// Auditoria central: persiste os audit.Record do serviço (mensagens,
	// notificações) e as negações de autorização do pkg/auth em audit.events.
	audit.SetDefault(audit.NewRecorder(pgDB))

	// Dependencies
	emailService := service.NewEmailService(cfg.Email)
	notifService := service.NewNotificationService(emailService)

	// WhatsApp Business (Meta Cloud API) — Issue #443. Opcional: quando
	// WHATSAPP_ACCESS_TOKEN / WHATSAPP_PHONE_NUMBER_ID não estão setados,
	// retorna (nil, nil) e o NotificationService continua aceitando email
	// mas recusa WHATSAPP. Erro real (config parcial inconsistente) sobe
	// e mata o boot — sinal claro de erro de deploy.
	if waSvc, err := service.NewWhatsAppServiceFromEnv(); err != nil {
		log.Fatalf("Failed to initialize WhatsApp service: %v", err)
	} else if waSvc != nil {
		notifService.WithWhatsAppService(waSvc)
		log.Printf("WhatsApp Business dispatcher enabled (template=%v)", os.Getenv("WHATSAPP_TEMPLATE_NAME") != "")
	} else {
		log.Println("WhatsApp Business dispatcher not configured; TypeWhatsApp notifications will fail with descriptive error")
	}

	// SMS Zenvia — Issue #444. Mesma filosofia do WhatsApp:
	// SMS_ZENVIA_API_TOKEN / SMS_ZENVIA_FROM ausentes → (nil, nil), o
	// canal fica off, TypeSMS notifications falham com erro descritivo
	// (callers de financeiro/locação decidem se logam SkippedNote ou
	// degradam pra outro canal). Config parcial mata o boot.
	if smsSvc, err := service.NewSMSServiceFromEnv(); err != nil {
		log.Fatalf("Failed to initialize SMS service: %v", err)
	} else if smsSvc != nil {
		notifService.WithSMSService(smsSvc)
		log.Printf("SMS Zenvia dispatcher enabled (from=%s)", os.Getenv("SMS_ZENVIA_FROM"))
	} else {
		log.Println("SMS Zenvia dispatcher not configured; TypeSMS notifications will fail with descriptive error")
	}

	notifHandler := handlers.NewNotificationHandler(notifService)

	hub := service.NewHub()
	go hub.Run()

	wsHandler := handlers.NewWebSocketHandler(hub, cfg.CORS.AllowedOrigins)

	// Setup Storage
	storageService, err := storage.NewFromEnv()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	inAppRepo := repository.NewInAppNotificationPostgresRepository(pgDB)
	inAppService := service.NewInAppNotificationService(inAppRepo, hub)
	inAppHandler := handlers.NewInAppNotificationHandler(inAppService)
	sentExternalsRepo := repository.NewSentExternalsPostgresRepository(pgDB)
	// Notificações transacionais por e-mail (beta — docs/features/comunicacao/
	// notificacoes-email-beta.md). O notifier aplica opt-out, presença,
	// cooldown e idempotência e é consumido pelos braços de e-mail do
	// EventConsumer. Kill-switch global em COMM_EMAIL_ENABLED (off por
	// padrão); categorias em COMM_EMAIL_*.
	emailPrefsRepo := repository.NewEmailPrefsPostgresRepository(pgDB)
	recipientResolver := repository.NewEmailRecipientPostgresResolver(pgDB)
	emailNotifier := service.NewEmailNotifier(emailService, emailPrefsRepo, sentExternalsRepo, hub, cfg.Email)
	emailPrefsService := service.NewEmailPrefsService(emailPrefsRepo)
	emailPrefsHandler := handlers.NewEmailPrefsHandler(emailPrefsService)
	if cfg.Email.TransactionalEmailEnabled {
		log.Printf("Transactional email notifications enabled (cooldown=%dmin)", cfg.Email.EmailCooldownMinutes)
	} else {
		log.Println("Transactional email notifications disabled (COMM_EMAIL_ENABLED=false)")
	}

	// Lembrete de mensagens não lidas (beta F3): motor com ticker próprio e
	// advisory lock multi-pod. Sempre sobe; o envio é gateado pelas flags e
	// pelo EmailNotifier (COMM_EMAIL_CHAT_REMINDERS + cooldown).
	chatReminderEngine := service.NewChatReminderEngine(
		repository.NewChatReminderPostgresRepository(pgDB),
		recipientResolver,
		emailNotifier,
		hub,
		time.Duration(cfg.Email.EmailChatReminderAfterMinutes)*time.Minute,
	).WithDB(pgDB)
	chatReminderEngine.Start()
	defer chatReminderEngine.Stop()

	msgRepo := repository.NewMessagePostgresRepository(pgDB)
	msgService := service.NewMessageService(msgRepo, hub, storageService, inAppService)
	msgHandler := handlers.NewMessageHandler(msgService)

	// Event Bus Setup
	var rb *events.RabbitMQEventBus
	var eventBus events.EventBus
	if cfg.RabbitMQ.URI != "" {
		rb, err = events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
		if err != nil {
			log.Printf("Warning: Failed to connect to RabbitMQ: %v", err)
		} else {
			defer rb.Close()
			eventBus = rb

			commEventHandler := service.NewCommEventHandler(msgService, inAppService)

			// Subscribe to domain events with durable named queues
			// deal.created is handled by EventConsumer (comm_deal_created)
			// with idempotent EnsureChannelForDeal — do NOT also subscribe
			// HandleDealCreated here, or it creates a duplicate channel.
			_ = eventBus.SubscribeWithQueue("toca-comm.lead.created", "lead.created", commEventHandler.HandleLeadCreated)
			// lead.assigned: subscription canônica fica em
			// EventConsumer.Start (event_consumer.go:713 — comm_lead_assigned)
			// que é idempotente (DeriveNotificationID), faz dispatch de
			// canais externos (whatsapp/sms/email) e usa InAppTypeLeadAssigned
			// (frontend NotificationBell trata com CTA "Ver Lead"). O handler
			// antigo CommEventHandler.HandleLeadAssigned ficou redundante e
			// causava double-notification. Issue #408 — Sprint 1.
			_ = eventBus.SubscribeWithQueue("toca-comm.portal.lead_received", "portal.lead_received", commEventHandler.HandlePortalLeadReceived)
			// Vitrine (ADR 0006): lead do site e transições da projeção.
			// Sem estas filas o exchange não tem rota e o outbox do toca-sites
			// fica em NO_ROUTE/PENDING para sempre.
			_ = eventBus.SubscribeWithQueue("toca-comm.site.lead_received", "site.lead.received", commEventHandler.HandleSiteLeadReceived)
			_ = eventBus.SubscribeWithQueue("toca-comm.site.listing_published", "site.listing.published", commEventHandler.HandleSiteListingEvent)
			_ = eventBus.SubscribeWithQueue("toca-comm.site.listing_unpublished", "site.listing.unpublished", commEventHandler.HandleSiteListingEvent)
			// broker.updated → broadcast WS "broker_changed" para clientes refazerem fetch.
			_ = eventBus.SubscribeWithQueue("toca-comm.broker.updated", "broker.updated", commEventHandler.HandleBrokerChanged)
			// roulette.config_updated → broadcast WS "roulette_config_updated"
			// para que outros admins editando a mesma página recebam invalidação
			// de cache e toast (Wave P1.6 — fix do last-write-wins silencioso).
			_ = eventBus.SubscribeWithQueue("toca-comm.roulette.config_updated", "roulette.config_updated", commEventHandler.HandleRouletteConfigUpdated)

			// Auth Events
			authEventHandler := service.NewAuthEventHandler(emailService, inAppService)
			_ = eventBus.SubscribeWithQueue("toca-comm.auth.user.registered", "auth.user.registered", authEventHandler.HandleUserRegistered)
			_ = eventBus.SubscribeWithQueue("toca-comm.auth.password_reset.requested", "auth.password_reset.requested", authEventHandler.HandlePasswordResetRequested)
			_ = eventBus.SubscribeWithQueue("toca-comm.auth.invitation.created", "auth.invitation.created", authEventHandler.HandleInvitationCreated)
			_ = eventBus.SubscribeWithQueue("toca-comm.auth.otp.requested", "auth.otp.requested", authEventHandler.HandleOTPRequested)
			_ = eventBus.SubscribeWithQueue("toca-comm.auth.client.invited", "auth.client.invited", authEventHandler.HandleClientInvited)
			_ = eventBus.SubscribeWithQueue("toca-comm.client.visit.requested", "client.visit.requested", authEventHandler.HandleClientVisitRequested)

			eventConsumer := service.NewEventConsumer(eventBus, msgService, inAppService).
				WithSentExternalsRepo(sentExternalsRepo).
				WithEmailService(emailService).
				WithEmailNotifier(emailNotifier).
				WithRecipientResolver(recipientResolver)
			if err := eventConsumer.Start(context.Background()); err != nil {
				log.Printf("Failed to start event consumer: %v", err)
			}

			rb.StartDLQConsumers()
		}
	}

	// Setup Router
	r := routes.SetupRouter(cfg, msgHandler, notifHandler, inAppHandler, emailPrefsHandler, wsHandler)
	r.Use(pkgmw.RequestID())
	r.Use(tracing.Middleware("toca-comm"))
	r.Use(pkgmw.RequestTimeout(0))
	if rb != nil {
		events.RegisterDLQAdminRoutes(r, rb)
	}
	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(r, "toca-comm", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(r, "toca-comm", deep)

	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "./uploads"
	}
	r.StaticFS("/uploads", newProtectedUploadFS(storageDir))

	// Start Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Starting Communication service on port %s", cfg.Port)
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

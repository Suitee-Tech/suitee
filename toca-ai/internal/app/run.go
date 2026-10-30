package app

import (
	"context"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/health"
	"github.com/oginoino/toca/pkg/logger"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/tracing"
	"github.com/oginoino/toca/toca-ai/internal/handlers"
	"github.com/oginoino/toca/toca-ai/internal/repositories"
	"github.com/oginoino/toca/toca-ai/internal/services"
)

func Run() {
	shutdown, err := tracing.InitProvider("toca-ai")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}

	cfg := config.LoadBaseConfig()
	logger.Setup(cfg.Logger)

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pgDB.Close()

	// Auditoria central: persiste as negações de autorização do pkg/auth em
	// audit.events (o serviço em si delega mutações ao gateway/MCP, que já
	// auditam na origem).
	audit.SetDefault(audit.NewRecorder(pgDB))

	deepseekAPIKey := config.GetEnv("DEEPSEEK_API_KEY", "")
	deepseekBaseURL := config.GetEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1")
	deepseekModel := services.NormalizeDeepseekModel(config.GetEnv("DEEPSEEK_MODEL", services.DefaultDeepseekModel))
	mcpServerURL := config.GetEnv("MCP_SERVER_URL", "http://toca-mcp:8090")
	ocrServiceURL := config.GetEnv("OCR_SERVICE_URL", "http://toca-ocr:5001")
	maxIterations := config.GetEnvInt("AI_MAX_ITERATIONS", 5)
	timeoutSeconds := config.GetEnvInt("AI_REQUEST_TIMEOUT_SECONDS", 120)

	if cfg.AppMode == "release" {
		if deepseekAPIKey == "" {
			logger.Error("missing required env var", "key", "DEEPSEEK_API_KEY")
			os.Exit(1)
		}
		if cfg.Database.ServiceURI == "" {
			logger.Error("missing required config", "key", "DATABASE_URI_SERVICE")
			os.Exit(1)
		}
		if cfg.JWT.SupabaseJWKSURL == "" {
			logger.Error("missing required config", "key", "SUPABASE_JWKS_URL")
			os.Exit(1)
		}
	}

	timeout := time.Duration(timeoutSeconds) * time.Second
	deepseek := services.NewDeepseekClient(deepseekAPIKey, deepseekBaseURL, deepseekModel, timeout)
	mcpClient := services.NewMCPClient(mcpServerURL)
	prompts := services.NewPromptBuilder()
	repo := repositories.NewConversationPostgresRepository(pgDB)
	toolInvRepo := repositories.NewToolInvocationPostgresRepository(pgDB)
	// P1/A8: tool de escrita do agente exige a permissão do domínio na matriz do
	// tenant (a lista de rotas do MCP é grossa: lê/escreve).
	chatSvc := services.NewChatService(deepseek, mcpClient, prompts, repo, toolInvRepo, maxIterations, 2048).
		WithPermissionProvider(auth.NewCorePermissionProvider(clients.NewCoreClient()))

	ocrClient := services.NewOCRClient(ocrServiceURL)
	extractionSvc := services.NewExtractionService(ocrClient, deepseek, services.NewExtractionPromptBuilder(), deepseekModel)

	chatHandler := handlers.NewChatHandler(chatSvc, mcpClient)
	convHandler := handlers.NewConversationHandler(repo)
	suggestionsHandler := handlers.NewSuggestionsHandler(deepseek, mcpClient)
	extractionHandler := handlers.NewExtractionHandler(extractionSvc)
	r := handlers.SetupRouter(cfg, chatHandler, convHandler, suggestionsHandler, extractionHandler)
	r.Use(pkgmw.RequestID())
	r.Use(tracing.Middleware("toca-ai"))
	r.Use(pkgmw.RequestTimeout(0))

	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	health.RegisterGin(r, "toca-ai", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	health.RegisterDeepGin(r, "toca-ai", deep)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Starting AI service on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

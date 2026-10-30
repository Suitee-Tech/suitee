package routes

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/apidocs"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/validation"
	_ "github.com/oginoino/toca/toca-signature/docs"
	"github.com/oginoino/toca/toca-signature/internal/api/v1/handlers"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	"github.com/oginoino/toca/toca-signature/internal/service"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Deps groups the runtime dependencies that need to be wired in main.go and
// passed in here. Keeping it small (rather than re-deriving from cfg/pgDB)
// lets tests construct the router without spinning up Postgres.
type Deps struct {
	Repo            envelope.Repository
	Creds           credential.CredentialProvider
	WebhookService  *service.WebhookService
	EnvelopeService *service.EnvelopeService
	// CredentialAdmin é o provider por tenant do P1-14 (gravação/rotação
	// via endpoint admin). Nil = endpoints administrativos desligados.
	CredentialAdmin *credential.DBCredentialProvider
}

func SetupRouter(cfg config.BaseConfig, pgDB *database.PostgresDB, deps Deps) *gin.Engine {
	if err := auth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	auth.LogServiceKeyRollout()

	_ = validation.InitGin()

	r := gin.New()
	// P2/A16: política única de proxies confiáveis (pkg/middleware). Sem
	// TRUSTED_PROXIES nenhum proxy é confiado — falha fechada —, o que importa
	// aqui porque a allowlist de IP do webhook do Clicksign e o limite do
	// RequireInternalKey leem c.ClientIP(). Os stacks de release declaram os
	// CIDRs da rede do compose.
	pkgmw.ApplyTrustedProxies(r)
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(metrics.GinMiddleware("toca-signature"))

	r.GET("/metrics", metrics.Handler())

	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-signature"})
	})

	envelopeHandler := handlers.NewEnvelopeHandler(deps.Repo, deps.EnvelopeService)
	webhookHandler := handlers.NewWebhookHandler(deps.WebhookService, deps.Creds, parseAllowedIPs())
	configHandler := handlers.NewConfigHandler(deps.Creds)

	// Public webhook — NO JWT, gated by HMAC + IP allowlist inside the handler.
	r.POST("/webhooks/clicksign", webhookHandler.Receive)

	// Internal routes — usadas pelo toca-deal (SignatureClient).
	//
	// P1/A3: `InternalOrUserMiddleware` aceita a chave interna (workers/serviço)
	// OU o JWT do usuário que originou a chamada. Com o JWT presente, o tenant é
	// derivado do TOKEN — antes o handler confiava no `?tenant_id=` que o
	// chamador escolhia, e com a chave compartilhada isso significava acesso a
	// qualquer tenant. O cliente interno (pkg/clients.BaseClient) passou a
	// encaminhar o Authorization.
	internal := r.Group("/api/v1/signature/internal")
	internal.Use(auth.InternalOrUserMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
	// Leitura/escrita de envelope por contrato precisa do pool de serviço: a
	// chamada interna não traz claims de JWT e a RLS de signature.envelopes
	// esconde a linha (404 "envelope not found for contract"), o que fazia o
	// toca-deal descartar todo signer_signed/envelope_finished com "signature
	// envelope is not synchronized".
	internal.Use(internalServiceTx(pgDB))
	{
		internal.GET("/envelopes/by-contract/:contractId", envelopeHandler.GetByContractInternal)
		internal.POST("/envelopes/by-contract/:contractId", envelopeHandler.CreateForContractInternal)
		// Refresh the cached signed-document URL by hitting Clicksign's
		// `GET /envelopes/:id/documents`. Used by the toca-deal
		// backfill (DocumentService.BackfillUnstoredSignedPDFs) to
		// recover documents stuck because the original
		// `envelope_finished` webhook came without the URL ready.
		internal.POST("/envelopes/by-contract/:contractId/refresh-signed-url",
			envelopeHandler.RefreshSignedDocumentURLInternal)
	}

	// P0/A11: o plano de assinatura é de operador. O portal do cliente consome
	// a assinatura por /client-portal/deals/:id/signature (toca-deal), não por
	// aqui — o token de `cliente` não tem o que fazer nesta superfície.
	//
	// O Use vem ANTES de qualquer registro do grupo de propósito: em gin a
	// cadeia é congelada no registro, então middleware aplicado depois não vale
	// para as rotas já registradas (era o caso do swagger, que ficava sem auth).
	v1 := r.Group("/api/v1")
	v1.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), auth.RejectFunctionalRole("cliente"))
	// P2/M16: o gate por ambiente é do pkg/apidocs; a autenticação continua
	// valendo quando a documentação é montada (Use acima).
	apidocs.Register(v1, "/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	permissionProvider := auth.NewCorePermissionProvider(clients.NewCoreClient())

	signature := v1.Group("/signature")
	{
		signature.GET("/config",
			auth.RequirePermission(permissionProvider, "contracts:read"),
			configHandler.Get)
		signature.GET("/envelopes",
			auth.RequirePermission(permissionProvider, "contracts:read"),
			envelopeHandler.List)
		signature.GET("/envelopes/:id",
			auth.RequirePermission(permissionProvider, "contracts:read"),
			envelopeHandler.GetByID)
		signature.GET("/envelopes/by-contract/:contractId",
			auth.RequirePermission(permissionProvider, "contracts:read"),
			envelopeHandler.GetByContract)
		signature.GET("/envelopes/:id/events",
			auth.RequirePermission(permissionProvider, "contracts:read"),
			envelopeHandler.ListEvents)
		signature.POST("/envelopes/:id/cancel",
			auth.RequirePermission(permissionProvider, "contracts:write"),
			envelopeHandler.Cancel)
		signature.POST("/envelopes/:id/resend",
			auth.RequirePermission(permissionProvider, "contracts:write"),
			envelopeHandler.Resend)
		signature.POST("/envelopes/:id/reconcile",
			auth.RequirePermission(permissionProvider, "contracts:write"),
			envelopeHandler.Reconcile)
		signature.POST("/envelopes/:id/signers",
			auth.RequirePermission(permissionProvider, "contracts:write"),
			envelopeHandler.AddSigner)
		signature.PATCH("/envelopes/:id/signers/:signerId",
			auth.RequirePermission(permissionProvider, "contracts:write"),
			envelopeHandler.EditSigner)
	}

	// P1-14: credenciais Clicksign por tenant. Guarda settings:manage_integrations
	// (admin). Quando o provider por tenant não está wired (tests), o grupo
	// responde 503 em vez de expor segredos.
	if deps.CredentialAdmin != nil {
		credentialHandler := handlers.NewCredentialHandler(deps.CredentialAdmin)
		admin := signature.Group("/admin/clicksign-credentials")
		admin.Use(auth.RequirePermission(permissionProvider, "settings:manage_integrations"))
		{
			admin.PUT("", credentialHandler.StoreCredential)
			admin.GET("", credentialHandler.GetCredential)
			admin.GET("/rotations", credentialHandler.ListCredentialRotations)
		}
	}

	return r
}

func parseAllowedIPs() []string {
	raw := os.Getenv("CLICKSIGN_WEBHOOK_ALLOWED_IPS")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

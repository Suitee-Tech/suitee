package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/apidocs"
	pkgauth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/validation"
	_ "github.com/oginoino/toca/toca-comm/docs"
	"github.com/oginoino/toca/toca-comm/internal/api/v1/handlers"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRouter(cfg config.BaseConfig, msgHandler *handlers.MessageHandler, notifHandler *handlers.NotificationHandler, inappNotifHandler *handlers.InAppNotificationHandler, emailPrefsHandler *handlers.EmailPrefsHandler, wsHandler *handlers.WebSocketHandler) *gin.Engine {
	// Initialize JWKS for Supabase JWT validation
	if err := pkgauth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	pkgauth.LogServiceKeyRollout()

	_ = validation.InitGin()

	r := gin.New()
	r.MaxMultipartMemory = 8 << 20 // 8 MB
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(metrics.GinMiddleware("toca-comm"))

	r.GET("/metrics", metrics.Handler())

	// Health check (no auth)
	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-comm"})
	})

	// Auth middleware (shared across services)
	authMiddleware := pkgauth.InternalOrMiddleware(pkgauth.Config{
		SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL,
	})

	// API V1
	v1 := r.Group("/api/v1")

	// Swagger — P2/M16: só fora de release (pkg/apidocs).
	apidocs.Register(v1, "/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// WhatsApp webhook — Issue #443 follow-up. NÃO usa JWT (Meta envia
	// sem auth header) — protegido por signature HMAC-SHA256 com
	// app_secret e por token compartilhado no GET verify. Sob /api/v1
	// para herdar o request_id middleware mas fora de authMiddleware.
	whatsappWebhook := handlers.NewWhatsAppWebhookHandler()
	v1.GET("/webhooks/whatsapp", whatsappWebhook.Verify)
	v1.POST("/webhooks/whatsapp", whatsappWebhook.Receive)

	// SMS webhook (Zenvia) — Issue #444. Mesma postura do WhatsApp:
	// sem JWT, protegido por HMAC-SHA1 com SMS_ZENVIA_WEBHOOK_SECRET.
	// Zenvia não faz verification handshake — só POST de delivery
	// status. Mesma motivação de ficar sob /api/v1 (request_id).
	smsWebhook := handlers.NewSMSWebhookHandler()
	v1.POST("/webhooks/sms", smsWebhook.Receive)

	// Internal routes — usadas por toca-crm e toca-deal (CommClient) e pelo
	// toca-financial (notificação in-app de cobrança).
	//
	// P1/A3: `InternalOrUserMiddleware` aceita a chave interna (workers/serviço)
	// OU o JWT do usuário que originou a chamada. Com o JWT presente, o tenant é
	// derivado do TOKEN — antes os handlers confiavam no `tenant_id` que o
	// chamador mandava no corpo, e com a chave compartilhada isso significava
	// acesso a qualquer tenant.
	internalComm := v1.Group("/comm/internal")
	internalComm.Use(pkgauth.InternalOrUserMiddleware(pkgauth.Config{
		SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL,
	}))
	{
		// P1/A3: o envio de e-mail/WhatsApp/SMS é primitivo interno — escolhe
		// destinatário, assunto e corpo. Ficava em `POST /api/v1/notifications/send`,
		// caminho que o gateway proxia e que NÃO tem o segmento `internal`, então a
		// denylist da borda (P0/C10) não o cobria: quem tivesse a INTERNAL_API_KEY
		// disparava mensagens em nome da plataforma pela internet. Sob
		// `/comm/internal/` a borda responde 404 e o caminho legítimo (serviço →
		// `COMM_SERVICE_URL`, na rede do compose) continua igual.
		internalComm.POST("/notifications/send", notifHandler.Send)
		internalComm.POST("/channels/ensure", msgHandler.EnsureChannelForDeal)
		internalComm.POST("/channels/direct", msgHandler.EnsureDirectChannel)
		internalComm.GET("/channels", msgHandler.ListChannelsByParticipant)
		internalComm.POST("/notifications/:accountId", inappNotifHandler.PushNotification)
	}

	// In-app notifications
	inappNotifs := v1.Group("/notifications/inapp", authMiddleware)
	{
		inappNotifs.GET("", inappNotifHandler.List)
		inappNotifs.GET("/unread-count", inappNotifHandler.GetUnreadCount)
		inappNotifs.PUT("/read-all", inappNotifHandler.MarkAllAsRead)
		inappNotifs.PUT("/:id/read", inappNotifHandler.MarkAsRead)
	}

	// Preferências de e-mail transacional (beta F4): qualquer usuário
	// autenticado gerencia as próprias preferências — a autorização é a
	// identidade do token (user_id), não o tenant.
	emailPrefs := v1.Group("/notifications/email-prefs", authMiddleware)
	{
		emailPrefs.GET("", emailPrefsHandler.List)
		emailPrefs.PUT("", emailPrefsHandler.Update)
	}

	// WebSocket endpoint
	v1.GET("/ws", authMiddleware, wsHandler.ServeWS)

	// P1/A8: busca, autocomplete e métricas do chat não tinham verificação de
	// permissão — qualquer autenticado não-`cliente` lia a busca de mensagens de
	// todos os canais do tenant. `messages:read` espelha a população da tela
	// (`/mensagens` exige `module:crm`).
	permissionProvider := pkgauth.NewCorePermissionProvider(clients.NewCoreClient())
	requireMessagesRead := pkgauth.RequirePermission(permissionProvider, "messages:read")

	// Autocomplete endpoint
	v1.GET("/autocomplete", authMiddleware, requireMessagesRead, msgHandler.Autocomplete)

	// Authenticated messaging endpoints
	v1.GET("/messages/search", authMiddleware, requireMessagesRead, msgHandler.SearchMessages)

	// Superfícies do TENANT: criar e listar todos os canais. Continuam fechadas
	// ao `cliente` do portal (P0/A11).
	tenantChannels := v1.Group("/channels", authMiddleware, pkgauth.RejectFunctionalRole("cliente"))
	{
		tenantChannels.POST("", msgHandler.CreateChannel)
		tenantChannels.GET("", msgHandler.ListChannels)
		// Administração de participantes: decide QUEM entra na conversa —
		// superfície de tenant, não do canal. Além do papel, o serviço exige que
		// o novo participante pertença ao tenant do canal (A9).
		tenantChannels.POST("/:id/participants", msgHandler.AddParticipant)
		tenantChannels.DELETE("/:id/participants/:participantId", msgHandler.RemoveParticipant)
		tenantChannels.PATCH("/:id/status", msgHandler.UpdateStatus)
	}

	// Superfícies do CANAL: autorizadas por PARTICIPAÇÃO (A9), que é validada no
	// serviço de forma fail-closed. Aqui não há `RejectFunctionalRole("cliente")`
	// de propósito: o cliente do portal é participante do canal do seu negócio e
	// usa exatamente estas rotas (`usePortalChannels` → `/channels/:id/messages`,
	// `/read`, reações, upload, pin/mute). Rejeitar o papel no grupo inteiro
	// apagava o chat do portal sem ganho de segurança — quem não participa não
	// passa, e quem participa só vê o canal em que está.
	channels := v1.Group("/channels", authMiddleware)
	{
		channels.GET("/mine", msgHandler.ListMyChannels)
		channels.GET("/:id", msgHandler.GetChannel)
		channels.PATCH("/:id", msgHandler.UpdateChannel)
		channels.POST("/:id/messages", msgHandler.SendMessage)
		channels.POST("/:id/messages/upload", msgHandler.UploadAttachment)
		channels.GET("/:id/messages/attachments/:attachmentId/download", msgHandler.DownloadAttachment)
		channels.GET("/:id/messages", msgHandler.GetMessages)
		channels.PATCH("/:id/messages/:messageId", msgHandler.EditMessage)
		channels.DELETE("/:id/messages/:messageId", msgHandler.DeleteMessage)
		channels.POST("/:id/messages/:messageId/reactions", msgHandler.AddReaction)
		channels.DELETE("/:id/messages/:messageId/reactions/:emoji", msgHandler.RemoveReaction)
		channels.PATCH("/:id/messages/:messageId/pin", msgHandler.TogglePinMessage)
		channels.POST("/:id/typing", msgHandler.SendTyping)
		channels.POST("/:id/read", msgHandler.MarkRead)
		channels.PATCH("/:id/pin", msgHandler.TogglePinChannel)
		channels.PATCH("/:id/mute", msgHandler.ToggleMuteChannel)
	}

	// Templates endpoints
	// Templates de MENSAGEM (não confundir com `templates:*` do catálogo, que é
	// modelo de CONTRATO): ver exige `messages:read`, criar exige `messages:write`.
	templates := v1.Group("/templates", authMiddleware, pkgauth.RejectFunctionalRole("cliente"))
	{
		templates.POST("", pkgauth.RequirePermission(permissionProvider, "messages:write"), msgHandler.CreateTemplate)
		templates.GET("", requireMessagesRead, msgHandler.ListTemplates)
	}

	// Metrics endpoint
	v1.GET("/metrics", authMiddleware, requireMessagesRead, msgHandler.GetMetrics)

	return r
}

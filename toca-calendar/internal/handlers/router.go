package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-calendar/internal/services"
)

func SetupRouter(cfg config.BaseConfig, appointmentService *services.AppointmentService) *gin.Engine {
	if err := auth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	auth.LogServiceKeyRollout()

	_ = validation.InitGin()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(metrics.GinMiddleware("toca-calendar"))

	r.GET("/metrics", metrics.Handler())

	r.Any("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "toca-calendar",
		})
	})

	appointmentHandler := NewAppointmentHandler(appointmentService)

	v1 := r.Group("/api/v1")

	// Internal routes — usadas pelo toca-crm e toca-deal (CalendarClient).
	//
	// P1/A3: `InternalOrUserMiddleware` aceita a chave interna (workers/serviço)
	// OU o JWT do usuário que originou a chamada. Com o JWT presente, o tenant é
	// derivado do TOKEN — antes os handlers confiavam no `?tenant_id=` (ou no
	// `tenant_id` do corpo) que o chamador escolhia, e com a chave compartilhada
	// isso significava acesso a qualquer tenant.
	internalGroup := v1.Group("/calendar/internal")
	internalGroup.Use(auth.InternalOrUserMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
	{
		internalGroup.POST("/appointments", appointmentHandler.InternalCreate)
		internalGroup.GET("/appointments/:id", appointmentHandler.InternalGetByID)
		internalGroup.PUT("/appointments/:id", appointmentHandler.InternalUpdate)
		internalGroup.DELETE("/appointments/:id", appointmentHandler.InternalDelete)
		internalGroup.GET("/deals/:id/appointments", appointmentHandler.ListByDeal)
	}

	v1.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), auth.RejectFunctionalRole("cliente"))

	// P1/A8: as 5 rotas de agenda não tinham NENHUMA verificação de permissão —
	// qualquer autenticado não-`cliente` (inclusive `marketing`, documentado sem
	// acesso a deals) criava, editava e apagava compromissos de qualquer corretor.
	//
	// O compromisso nasce do negócio/esteira (a tela é `module:crm` e o
	// `CalendarContext` opera a agenda; o fluxo de visita do cliente passa por
	// `/client-portal/...`). Por isso o mapa é o do catálogo para deals:
	//
	//	ver      → `deals:read`
	//	escrever → `deals:write`
	//
	// `deals:read` foi alinhado à matriz (inclui financeiro e auditor, que a
	// documentação lista como leitores da esteira).
	permissionProvider := auth.NewCorePermissionProvider(clients.NewCoreClient())
	appointments := v1.Group("/appointments")
	{
		appointments.POST("", auth.RequirePermission(permissionProvider, "deals:write"), appointmentHandler.Create)
		appointments.GET("", auth.RequirePermission(permissionProvider, "deals:read"), appointmentHandler.List)
		appointments.GET("/:id", auth.RequirePermission(permissionProvider, "deals:read"), appointmentHandler.GetByID)
		appointments.PUT("/:id", auth.RequirePermission(permissionProvider, "deals:write"), appointmentHandler.Update)
		appointments.DELETE("/:id", auth.RequirePermission(permissionProvider, "deals:write"), appointmentHandler.Delete)
		appointments.GET("/:id/calendar.ics", auth.RequirePermission(permissionProvider, "deals:read"), appointmentHandler.CalendarFile)
	}

	// Download assinado do e-mail: sem sessão, e por isso fora do grupo `v1`
	// (que exige autenticação). O token assinado diz o tenant e a validade, e o
	// compromisso é revalidado na leitura — não há dado de outro cliente aqui.
	// O caminho não fica sob `/appointments/` para não colidir com `/:id`.
	public := r.Group("/api/v1/calendar/public")
	{
		public.GET("/appointments/:id/calendar.ics", appointmentHandler.PublicCalendarFile)
	}

	return r
}

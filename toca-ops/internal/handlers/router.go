package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-ops/internal/services"
)

func SetupRouter(cfg config.BaseConfig, maintenanceService *services.MaintenanceService, keyService *services.KeyService, inspectionService *services.InspectionService, ticketService *services.TicketService, supplierService *services.SupplierService) *gin.Engine {
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
	r.Use(metrics.GinMiddleware("toca-ops"))

	r.GET("/metrics", metrics.Handler())

	maintenanceHandler := NewMaintenanceHandler(maintenanceService)
	keyHandler := NewKeyHandler(keyService)
	supplierHandler := NewSupplierHandler(supplierService)
	inspectionHandler := NewInspectionHandler(inspectionService)
	ticketHandler := NewTicketHandler(ticketService)
	permissionProvider := auth.NewCorePermissionProvider(clients.NewCoreClient())

	// P1/A12: o efeito financeiro da manutenção (cobrança do inquilino, dedução
	// do repasse do proprietário) passa a exigir permissão financeira.
	maintenanceHandler = maintenanceHandler.WithPermissionProvider(permissionProvider)

	r.Any("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "toca-ops",
		})
	})

	v1 := r.Group("/api/v1")

	// Internal routes — usadas pelo toca-deal (OpsClient), toca-crm e workers.
	//
	// P1/A3: `InternalOrUserMiddleware` aceita a chave interna (workers/serviço)
	// OU o JWT do usuário que originou a chamada. Com o JWT presente, o tenant é
	// derivado do TOKEN — antes os handlers confiavam no `?tenant_id=` (ou no
	// `tenant_id` do corpo) que o chamador escolhia, e com a chave compartilhada
	// isso significava acesso a qualquer tenant.
	internalGroup := v1.Group("/ops/internal")
	internalGroup.Use(auth.InternalOrUserMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
	{
		internalGroup.POST("/maintenance-tickets", maintenanceHandler.Create)
		internalGroup.GET("/properties/:id/maintenance-tickets", maintenanceHandler.ListByProperty)
		internalGroup.POST("/tickets", ticketHandler.Create)
		internalGroup.GET("/tickets/by-requester/:accountID", ticketHandler.ListByRequester)
		internalGroup.POST("/tickets/:id/comments", ticketHandler.AddComment)
		internalGroup.GET("/tickets/:id", ticketHandler.GetByIDInternal)
		internalGroup.GET("/inspections", inspectionHandler.ListByContractInternal)
	}

	secured := v1.Group("")
	secured.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), auth.RejectFunctionalRole("cliente"))

	// P1/A8: manutenção, chaves e fornecedores não tinham NENHUMA verificação de
	// permissão — qualquer autenticado não-`cliente` (inclusive `marketing` e
	// `financeiro`) criava, editava e apagava chamado de manutenção e cadastro de
	// prestador, e fazia retirada/devolução de chave.
	//
	// Os três são a superfície OPERACIONAL de Locação: a tela (`/manutencao`) exige
	// `module:rentals` + `rentals:inspect` no cliente, tem as abas Manutenção e
	// Vistoria, e o `SupplierPicker` só é renderizado nela. Por isso o guard é o
	// MESMO do grupo de vistorias deste serviço (`rentals:inspect`:
	// admin/gestor/operacoes/corretor) — não muda quem opera hoje e fecha o
	// "qualquer autenticado". Se chaves/fornecedores ganharem tela própria, cada um
	// pode merecer permissão própria; hoje não há consumidor (o `KeysContext` do
	// frontend não é usado por nenhuma página).
	requireRentalsInspect := auth.RequirePermission(permissionProvider, "rentals:inspect")

	maintenanceGroup := secured.Group("/maintenance")
	{
		maintenanceGroup.POST("", requireRentalsInspect, maintenanceHandler.Create)
		maintenanceGroup.GET("", requireRentalsInspect, maintenanceHandler.List)
		maintenanceGroup.GET("/:id", requireRentalsInspect, maintenanceHandler.GetByID)
		maintenanceGroup.PUT("/:id", requireRentalsInspect, maintenanceHandler.Update)
		maintenanceGroup.DELETE("/:id", requireRentalsInspect, maintenanceHandler.Delete)
		maintenanceGroup.PATCH("/:id/status", requireRentalsInspect, maintenanceHandler.ChangeStatus)
	}

	keysGroup := secured.Group("/keys")
	{
		keysGroup.POST("", requireRentalsInspect, keyHandler.Create)
		keysGroup.GET("", requireRentalsInspect, keyHandler.List)
		keysGroup.GET("/:id", requireRentalsInspect, keyHandler.GetByID)
		keysGroup.DELETE("/:id", requireRentalsInspect, keyHandler.Delete)
		keysGroup.POST("/:id/checkout", requireRentalsInspect, keyHandler.CheckOut)
		keysGroup.POST("/:id/checkin", requireRentalsInspect, keyHandler.CheckIn)
	}

	// Cadastro de profissionais/fornecedores: reutilizado por manutenção e
	// vistoria para acionar sempre o mesmo prestador.
	suppliersGroup := secured.Group("/suppliers")
	{
		suppliersGroup.POST("", requireRentalsInspect, supplierHandler.Create)
		suppliersGroup.GET("", requireRentalsInspect, supplierHandler.List)
		suppliersGroup.GET("/service-types", requireRentalsInspect, supplierHandler.ServiceTypes)
		suppliersGroup.GET("/:id", requireRentalsInspect, supplierHandler.GetByID)
		suppliersGroup.PUT("/:id", requireRentalsInspect, supplierHandler.Update)
		suppliersGroup.DELETE("/:id", requireRentalsInspect, supplierHandler.Delete)
	}

	// Vistorias usam `rentals:inspect` (auditoria C8/A8): é a permissão que o
	// menu de manutenção/locação do frontend já usa para exibir a tela, de modo
	// que o backend passa a exigir o mesmo direito que a UI assume. Sem guarda
	// alguma, qualquer autenticado não-`cliente` lia e regravava vistorias.
	inspectionsGroup := secured.Group("/inspections")
	{
		inspectionsGroup.POST("", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.Create)
		inspectionsGroup.GET("", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.List)
		inspectionsGroup.GET("/:id", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.GetByID)
		inspectionsGroup.PUT("/:id", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.Update)
		inspectionsGroup.DELETE("/:id", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.Delete)
		inspectionsGroup.POST("/:id/deductions", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.RecordDeductions)
		inspectionsGroup.POST("/:id/findings/:finding_id/photos", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.UploadFindingPhotos)
		inspectionsGroup.POST("/:id/generate-laudo", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.GenerateLaudo)
		inspectionsGroup.GET("/template", auth.RequirePermission(permissionProvider, "rentals:inspect"), inspectionHandler.GetTemplate)
	}

	ticketsGroup := secured.Group("/tickets")
	{
		ticketsGroup.POST("", auth.RequirePermission(permissionProvider, "tickets:write"), ticketHandler.Create)
		ticketsGroup.GET("", auth.RequirePermission(permissionProvider, "tickets:read"), ticketHandler.List)
		ticketsGroup.GET("/:id", auth.RequirePermission(permissionProvider, "tickets:read"), ticketHandler.GetByID)
		ticketsGroup.PATCH("/:id/status", auth.RequirePermission(permissionProvider, "tickets:write"), ticketHandler.UpdateStatus)
		ticketsGroup.PATCH("/:id/assign", auth.RequirePermission(permissionProvider, "tickets:write"), ticketHandler.Assign)
		ticketsGroup.POST("/:id/comments", auth.RequirePermission(permissionProvider, "tickets:write"), ticketHandler.AddComment)
	}

	return r
}

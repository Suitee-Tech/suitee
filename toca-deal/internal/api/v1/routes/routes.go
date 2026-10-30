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
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/pkg/virusscan"
	_ "github.com/oginoino/toca/toca-deal/docs"
	"github.com/oginoino/toca/toca-deal/internal/api/v1/handlers"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	repository "github.com/oginoino/toca/toca-deal/internal/repositories"
	service "github.com/oginoino/toca/toca-deal/internal/services"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Services exposes the wired service layer to main.go so it can register
// EventBus subscribers without duplicating the construction graph.
type Services struct {
	Contract *service.ContractService
	Document *service.DocumentService
	Deal     *service.DealService

	// Repos exported so projection-refresh subscribers in main.go can
	// query/Update directly without going through the service layer (they
	// only do bulk-update of snapshot fields, no business rule logic
	// applies).
	ContractRepo contract.Repository
	DealRepo     deal.Repository
}

func SetupRouter(cfg config.BaseConfig, pgDB *database.PostgresDB, eventBus events.EventBus, storageService storage.Service) (*gin.Engine, Services) {
	// Initialize JWKS for Supabase JWT validation
	if err := auth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	auth.LogServiceKeyRollout()

	_ = validation.InitGin()

	r := gin.New()
	r.MaxMultipartMemory = 8 << 20 // 8 MB
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(metrics.GinMiddleware("toca-deal"))

	r.GET("/metrics", metrics.Handler())

	// Health check (no auth)
	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-deal"})
	})

	// SECURITY (achado B8 da revisão de permissões de 2026-09-19): o mount
	// `r.Static("/uploads", "./uploads")` foi REMOVIDO. Ele servia o diretório
	// de uploads sem nenhuma autenticação — contratos, documentos de identidade
	// e comprovantes ficavam legíveis por quem conhecesse a URL (e o volume real
	// é /uploads, não ./uploads: o mount podia servir o diretório errado
	// conforme o WORKDIR da imagem). Os arquivos continuam acessíveis pelos
	// endpoints autenticados (documento, checklist e portal), que resolvem o
	// caminho local via storage.LocalUploadPath (uma única implementação,
	// confinada ao STORAGE_DIR).

	// Dependencies
	dealRepo := repository.NewDealPostgresRepository(pgDB)
	// O portal do cliente é cross-tenant: as operações do negócio dele rodam
	// no pool de serviço (BYPASSRLS), com autorização pelo vínculo do signatário.
	portalDealRepo := repository.NewPortalDealRepository(pgDB)
	propertyRepo := repository.NewPropertyPostgresRepository(pgDB)
	leadRepo := repository.NewLeadPostgresRepository(pgDB)
	dealService := service.NewDealService(dealRepo, propertyRepo, leadRepo, eventBus, storageService)
	dealService.WithPortalRepository(portalDealRepo)
	// Resolve the versioned, tenant-scoped approval gate from toca-auth. The
	// client provides bounded cache/stale behavior and DealService retains the
	// historical env/default threshold as an explicit availability fallback.
	dealService.WithApprovalPolicyProvider(clients.NewAuthClient())
	// crossRepo reads core.properties / crm.leads / core.partnerships so
	// DealService.Create can authorize a co-brokering deal against a
	// partner-shared resource (and publish partner.deal_initiated).
	dealService.WithCrossRepo(repository.NewCrossPostgresRepository(pgDB))
	dealHandler := handlers.NewDealHandler(dealService)

	contractRepo := repository.NewContractPostgresRepository(pgDB)
	signatureClients := []service.SignatureEnvelopeCreator{}
	if os.Getenv("SIGNATURE_SERVICE_URL") != "" {
		signatureClients = append(signatureClients, clients.NewSignatureClient())
	} else {
		logger.Warn("SIGNATURE_SERVICE_URL not configured; contract signature dispatch will rely on the event bus")
	}
	contractService := service.NewContractService(contractRepo, dealRepo, eventBus, pgDB, signatureClients...)
	// Wire the ops client so the contract termination endpoint can validate
	// "vistoria de saída concluída" when the operator passes
	// require_exit_inspection=true. Best-effort: when OPS_SERVICE_URL is
	// unset the checker is wired anyway but its HTTP call will fail —
	// the require flag should remain disabled in that environment.
	if os.Getenv("OPS_SERVICE_URL") != "" {
		contractService.WithExitInspectionChecker(clients.NewOpsClient())
	}
	contractHandler := handlers.NewContractHandler(contractService)
	// Wire contract-driven signer resync into deal service so MoveDealStatus
	// auto-heals stale projections instead of blocking the "all required
	// signers must sign" gate when the contract is already SIGNED.
	dealService.WithSignerSyncer(contractService)
	// dealHandler also needs contractService for the portal signature view —
	// it crosses domains (deal access check + contract + envelope projection).
	dealHandler.WithContractService(contractService)

	docRepo := repository.NewDocumentPostgresRepository(pgDB)
	docService := service.NewDocumentService(docRepo, storageService)
	// P1-13: antivírus no funil de uploads. CLAMAV_URL é obrigatório em
	// produção (compose); o scanner é fail-closed — sem wiring, TODO upload
	// é recusado (check-virus-scan garante o wiring no compose).
	if clamavAddr := strings.TrimSpace(os.Getenv("CLAMAV_URL")); clamavAddr != "" {
		docService.WithVirusScanner(virusscan.NewClamAVScanner(clamavAddr))
		dealService.WithVirusScanner(virusscan.NewClamAVScanner(clamavAddr))
	}
	// Espelha no painel de documentos os arquivos anexados aos itens do checklist
	// da esteira digital (o upload vive no metadata do negócio).
	dealService.WithChecklistDocumentSync(docService)
	// Mirror contract.Status onto the Document records linked to it so the
	// Central de Documentos screen reflects signature lifecycle changes.
	contractService.WithDocumentSyncer(docService)
	// Symmetric link: let DocumentService fall back to the contract's
	// Clicksign URL when GetDownloadURL is called before the persist
	// subscriber has cached the signed PDF locally. Without this the
	// preview shows an empty state during the (often seconds-long) race
	// between envelope_finished and the storage upload finishing.
	docService.SetContractLookup(contractService)
	// Wire the event bus so user uploads of identity/proof documents publish
	// document.uploaded, which the extraction subscriber consumes to run OCR+IA.
	docService.SetEventBus(eventBus)
	// Wire DocumentService into ContractHandler so GET
	// /contracts/:id/signed-file can resolve the persisted PDF URL.
	contractHandler.WithDocumentService(docService)
	// Wire DealService as the access checker so DocumentHandler can enforce
	// party-aware RBAC on deal-scoped document queries (Phase 1, item 2).
	// P1/A25: leitura de documentos do contrato passa a validar o escopo do
	// negócio do contrato (o ContractService é quem conhece esse vínculo).
	docHandler := handlers.NewDocumentHandler(docService, dealService).WithContractScope(contractService)

	templateRepo := repository.NewTemplatePostgresRepository(pgDB)
	templateService := service.NewTemplateService(templateRepo)
	templateHandler := handlers.NewTemplateHandler(templateService)

	permissionProvider := auth.NewCorePermissionProvider(clients.NewCoreClient())
	// Wire o mesmo provider no handler para o approval gate (#416) avaliar
	// dynamicamente se o caller tem permission `deals:approve` ao
	// movimentar deals de alto valor.
	dealHandler.WithPermissionProvider(permissionProvider)

	// API V1
	v1 := r.Group("/api/v1")

	internal := r.Group("/api/v1/deal/internal")
	// Credencial primeiro (RequireInternalKey), depois a transação de serviço —
	// as leituras internas são cross-tenant e chegam sem claims de JWT.
	internal.Use(auth.RequireInternalKey(), internalServiceTx(pgDB))
	{
		internal.GET("/contracts/:id", contractHandler.GetByIDInternal)
		internal.GET("/documents/:id/content", docHandler.GetContentInternal)
		// Consumido por toca-financial.FinancialPreviewService para
		// projetar charges/comissões/repasses antes da assinatura.
		// Bypassa broker scope; auth via internal API key.
		internal.GET("/deals/:id", dealHandler.GetByIDInternal)
	}

	// Swagger — P2/M16: só fora de release (pkg/apidocs).
	apidocs.Register(v1, "/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Apply Auth Middleware
	v1.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))

	// P1/A20 — a superfície pública por token (`/api/v1/portal/:token`, token =
	// base64("tenantID:dealID"), sem assinatura e sem expiração) foi REMOVIDA.
	//
	// O token não era segredo: quem conhecesse o par (tenant, deal) o forjava, o
	// tenant vinha do token (não do JWT) e o escopo de corretor era desligado
	// (`GetByID(tenantID, dealID, "")`); a única barreira era o e-mail do JWT
	// coincidir com um signatário. A capacidade já existe na superfície
	// AUTENTICADA do portal do cliente — `/api/v1/client-portal/deals/:id`,
	// `/deals/:id/checklist` (ler/baixar/enviar/party-fields) — onde o tenant sai
	// do token do Supabase e o vínculo do cliente com o negócio é verificado
	// (`findMyDeal`). O frontend não consumia os endpoints por token.
	// A borda também passou a recusar o caminho (gateway: retiredPaths).

	// Client portal routes (authenticated via JWT)
	clientPortal := v1.Group("/client-portal")
	clientPortal.Use(auth.RequireFunctionalRole("cliente"))
	{
		clientPortal.GET("/channels", dealHandler.ListMyChannels)
		clientPortal.GET("/deals/:id/channel", dealHandler.GetMyDealChannel)
		clientPortal.GET("/deals", dealHandler.GetMyDeals)
		clientPortal.GET("/leads", dealHandler.GetMyLeads)
		clientPortal.GET("/deals/:id", dealHandler.GetMyDeal)
		clientPortal.GET("/deals/:id/property", dealHandler.GetMyDealProperty)
		clientPortal.GET("/deals/:id/signature", dealHandler.GetMyDealSignature)
		clientPortal.GET("/deals/:id/recommendations", dealHandler.GetMyDealRecommendations)
		clientPortal.GET("/deals/:id/properties", dealHandler.SearchMyDealProperties)
		clientPortal.GET("/deals/:id/checklist", dealHandler.GetMyDealChecklist)
		clientPortal.POST("/deals/:id/checklist/:itemId/upload-url", dealHandler.GetMyChecklistItemUploadURL)
		clientPortal.POST("/deals/:id/checklist/:itemId/upload-complete", dealHandler.CompleteMyChecklistItemUpload)
		clientPortal.POST("/deals/:id/checklist/:itemId/upload", dealHandler.UploadMyChecklistItem)
		clientPortal.GET("/deals/:id/checklist/:itemId/download", dealHandler.DownloadMyChecklistItem)
		clientPortal.PATCH("/deals/:id/checklist/:itemId/party-fields", dealHandler.UpdateMyChecklistPartyFields)
		clientPortal.GET("/deals/:id/appointments", dealHandler.GetMyDealAppointments)
		clientPortal.POST("/deals/:id/appointments", dealHandler.CreateMyDealAppointment)
		clientPortal.PUT("/deals/:id/appointments/:appointmentId", dealHandler.UpdateMyDealAppointment)
		clientPortal.DELETE("/deals/:id/appointments/:appointmentId", dealHandler.CancelMyDealAppointment)
		clientPortal.GET("/deals/:id/appointments/:appointmentId/calendar.ics", dealHandler.DownloadMyDealAppointmentCalendar)
		clientPortal.GET("/deals/:id/charges", dealHandler.GetMyDealCharges)
		clientPortal.GET("/deals/:id/maintenance", dealHandler.GetMyDealMaintenanceTickets)
		clientPortal.POST("/deals/:id/maintenance", dealHandler.CreateMyDealMaintenanceTicket)
		clientPortal.POST("/deals/:id/maintenance/attachments/upload-url", dealHandler.GetMyDealMaintenanceAttachmentUploadURL)
		clientPortal.POST("/deals/:id/maintenance/attachments/upload-complete", dealHandler.CompleteMyDealMaintenanceAttachmentUpload)
		clientPortal.GET("/deals/:id/tickets", dealHandler.GetMyDealTickets)
		clientPortal.POST("/deals/:id/tickets", dealHandler.CreateMyDealTicket)
		clientPortal.GET("/tickets/:id", dealHandler.GetMyTicket)
		clientPortal.POST("/tickets/:id/comments", dealHandler.AddMyTicketComment)
		clientPortal.POST("/tickets/:id/attachments/upload-url", dealHandler.GetMyTicketAttachmentUploadURL)
		clientPortal.POST("/tickets/:id/attachments/upload-complete", dealHandler.CompleteMyTicketAttachmentUpload)
		clientPortal.GET("/deals/:id/favorites", dealHandler.GetMyDealFavorites)
		clientPortal.POST("/deals/:id/favorites", dealHandler.AddMyDealFavorite)
		clientPortal.DELETE("/deals/:id/favorites/:propertyId", dealHandler.RemoveMyDealFavorite)
		clientPortal.POST("/deals/:id/proposals/:proposalId/accept", dealHandler.AcceptMyDealProposal)
		clientPortal.POST("/deals/:id/proposals/:proposalId/reject", dealHandler.RejectMyDealProposal)
	}

	deals := v1.Group("/deals", auth.RejectFunctionalRole("cliente"))
	{
		deals.POST("", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.Create)
		deals.GET("", auth.RequirePermission(permissionProvider, "deals:read"), dealHandler.List)
		deals.GET("/stats", auth.RequirePermission(permissionProvider, "deals:read"), dealHandler.GetStats)
		deals.GET("/:id", auth.RequirePermission(permissionProvider, "deals:read"), dealHandler.GetByID)
		deals.PATCH("/:id/status", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.UpdateStatus)
		deals.DELETE("/:id", auth.RequirePermission(permissionProvider, "deals:delete"), dealHandler.Delete)
		// Persisted audit trail for the Esteira panel — replaces the local-only
		// log previously kept in CRMContext. Gated by the same permission
		// already used by AuditTrailPanel in the UI.
		deals.GET("/:id/audit-log", auth.RequirePermission(permissionProvider, "system:view_audit_trail"), dealHandler.GetAuditLog)

		// New routes for granular Esteira Digital
		deals.POST("/:id/proposals", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.AddProposal)
		// Operador/corretor agenda uma visita para o cliente (o cliente acompanha no portal).
		deals.POST("/:id/appointments", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.CreateDealAppointment)
		// Corretor recomenda um imóvel ao cliente (appears as "Sugestão do seu corretor").
		deals.PUT("/:id/recommended-property", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.SetRecommendedProperty)
		// Admin/operador aceita contraproposta do cliente (#424).
		// Requer `deals:approve` para evitar que corretor feche
		// negociações de qualquer valor sozinho — mesma proteção do
		// approval gate em UpdateStatus (#416).
		deals.POST("/:id/accept-counter-offer", auth.RequirePermission(permissionProvider, "deals:approve"), dealHandler.AcceptCounterOffer)
		deals.PUT("/:id/due-diligence", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.UpdateDueDiligence)
		deals.POST("/:id/signers", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.AddSigner)
		deals.PATCH("/:id/signers/:signerId", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.UpdateSignerStatus)
		deals.PUT("/:id/signers/:signerId", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.UpdateSigner)
		deals.DELETE("/:id/signers/:signerId", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.RemoveSigner)
		deals.POST("/:id/signers/:signerId/invite", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.InviteSigner)
		// Operator recovery hatch for the Esteira "Reconciliar" button.
		// Forces a re-sync of Deal.Signers from Contract.Signatories so a
		// drifted projection (the bug pattern in audit §8.5/§8.10) is
		// healed immediately, without depending on event redelivery.
		deals.POST("/:id/reconcile-signers", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.ReconcileSigners)
		// Broker participation (captador, visitador, fechamento) — single source of truth
		deals.POST("/:id/broker-participations", auth.RequirePermission(permissionProvider, "commissions:write"), dealHandler.AddBrokerParticipation)
		// Replace the entire broker_participations array — used by the
		// commission distribution UI (#420). Sum=100 and CONVERTED-lock
		// enforced server-side. `deals:write` is the same gate as Add,
		// but the handler reads scope-by-broker so corretor only
		// edita deals próprios; admin/gestor pegam sem filtro.
		deals.PUT("/:id/broker-participations", auth.RequirePermission(permissionProvider, "commissions:write"), dealHandler.SetBrokerParticipations)

		// Document checklist
		deals.PUT("/:id/checklist", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.InitChecklist)
		deals.POST("/:id/checklist", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.AddChecklistItem)
		deals.DELETE("/:id/checklist/:itemId", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.RemoveChecklistItem)
		deals.PATCH("/:id/checklist/:itemId", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.UpdateChecklistItemStatus)
		deals.POST("/:id/checklist/:itemId/solicit", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.SolicitChecklistItem)
		deals.GET("/:id/checklist/:itemId/download", auth.RequirePermission(permissionProvider, "deals:read"), dealHandler.DownloadChecklistItem)

		deals.GET("/:id/parties", auth.RequirePermission(permissionProvider, "deals:read"), dealHandler.ListDealParties)
		deals.POST("/:id/parties", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.AddParty)
		deals.PATCH("/:id/parties/:partyId", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.UpdatePartyRole)
		deals.PATCH("/:id/parties/:partyId/due-diligence", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.UpdatePartyDueDiligence)
		deals.PATCH("/:id/parties/:partyId/due-diligence/criteria/:criterion", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.UpdatePartyDueDiligenceCriterion)
		deals.DELETE("/:id/parties/:partyId", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.RemoveParty)
		deals.POST("/:id/parties/:partyId/set-primary", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.SetPrimaryParty)
		deals.POST("/:id/parties/:partyId/apply-extraction", auth.RequirePermission(permissionProvider, "deals:write"), dealHandler.ApplyExtraction)

		// Contracts within Deals context
		deals.GET("/:id/contracts", auth.RequirePermission(permissionProvider, "contracts:read"), contractHandler.ListByDeal)
		// Documents within Deals context — party-aware (admin/broker/party only).
		// See DocumentHandler.canAccessDeal for the gate logic.
		deals.GET("/:id/documents", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.ListByDeal)
		// Documentos enviados pelo cliente na esteira digital: o upload vive no
		// metadata do negócio e precisa aparecer no painel de documentos. A rota
		// existe para reconciliar o que foi enviado antes do espelho (idempotente).
		deals.POST("/backfill-checklist-documents", auth.RequirePermission(permissionProvider, "documents:write"), dealHandler.BackfillChecklistDocuments)
	}

	contracts := v1.Group("/contracts", auth.RejectFunctionalRole("cliente"))
	{
		contracts.POST("", auth.RequirePermission(permissionProvider, "contracts:write"), contractHandler.Create)
		contracts.GET("", auth.RequirePermission(permissionProvider, "contracts:read"), contractHandler.List)
		contracts.GET("/:id", auth.RequirePermission(permissionProvider, "contracts:read"), contractHandler.GetByID)
		contracts.PUT("/:id", auth.RequirePermission(permissionProvider, "contracts:write"), contractHandler.Update)
		contracts.DELETE("/:id", auth.RequirePermission(permissionProvider, "contracts:delete"), contractHandler.Delete)
		// P0/C12: assinar é uma capacidade própria (`contracts:sign`, matriz:
		// admin/gestor/operacoes) e não um efeito de `contracts:write`. O
		// handler ainda confronta a identidade do chamador com o signatário.
		contracts.POST("/:id/sign", auth.RequirePermission(permissionProvider, "contracts:sign"), contractHandler.Sign)
		contracts.POST("/:id/send-for-signature", auth.RequirePermission(permissionProvider, "contracts:write"), contractHandler.SendForSignature)
		contracts.POST("/:id/regenerate", auth.RequirePermission(permissionProvider, "contracts:write"), contractHandler.Regenerate)
		contracts.GET("/:id/signed-file", auth.RequirePermission(permissionProvider, "contracts:read"), contractHandler.GetSignedFile)
		// Rental management actions (Fase 1)
		contracts.POST("/:id/terminate", auth.RequirePermission(permissionProvider, "contracts:write"), contractHandler.Terminate)
		// Preview da multa de rescisão antecipada (#438). Read-only,
		// não muta; gate por contracts:read porque é "ver o número que
		// apareceria se eu terminasse hoje". Caller decide se POSTa
		// /terminate em seguida ou desiste.
		contracts.GET("/:id/termination-fee-preview", auth.RequirePermission(permissionProvider, "contracts:read"), contractHandler.PreviewTerminationFee)
		contracts.POST("/:id/readjust", auth.RequirePermission(permissionProvider, "contracts:write"), contractHandler.Readjust)
		// Amendment (aditivo) creates a new DRAFT contract linked to the
		// original via amends_contract_id. Operator then runs the signature
		// flow on the new contract via the existing endpoints.
		contracts.POST("/:id/amend", auth.RequirePermission(permissionProvider, "contracts:write"), contractHandler.Amend)
		// FASE 3 (integração total) — list every Document tied to the contract
		// (signed PDF, aditivos, laudos de vistoria, comprovantes de manutenção,
		// recibos de pagamento). Aggregator endpoint backing the
		// /locacao/:id "Documentos" tab.
		contracts.GET("/:id/documents", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.ListByContract)
	}

	templates := v1.Group("/contract-templates", auth.RejectFunctionalRole("cliente"))
	{
		templates.POST("", auth.RequirePermission(permissionProvider, "templates:write"), templateHandler.Create)
		templates.GET("", auth.RequirePermission(permissionProvider, "templates:read"), templateHandler.List)
		templates.GET("/:id", auth.RequirePermission(permissionProvider, "templates:read"), templateHandler.GetByID)
		templates.PUT("/:id", auth.RequirePermission(permissionProvider, "templates:write"), templateHandler.Update)
		templates.DELETE("/:id", auth.RequirePermission(permissionProvider, "templates:delete"), templateHandler.Delete)
	}

	// Checklist templates (Phase 4/W4) — read-only for now, returns the
	// built-in set; persistent per-tenant CRUD lands together with the
	// admin UI in a follow-up. Gated on `deals:read` since the FE only
	// consumes these inside the Esteira Documentation tab.
	v1.GET("/checklist-templates", auth.RejectFunctionalRole("cliente"), auth.RequirePermission(permissionProvider, "deals:read"), dealHandler.ListChecklistTemplates)

	// ── Documents: centralized single source of truth ──
	documents := v1.Group("/documents", auth.RejectFunctionalRole("cliente"))
	{
		// CRUD
		documents.POST("", auth.RequirePermission(permissionProvider, "documents:write"), docHandler.Create)
		documents.GET("", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.List)
		documents.GET("/search", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.Search)
		documents.GET("/tags", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.KnownTags)
		documents.GET("/:id", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.GetByID)
		documents.PUT("/:id", auth.RequirePermission(permissionProvider, "documents:write"), docHandler.Update)
		documents.DELETE("/:id", auth.RequirePermission(permissionProvider, "documents:delete"), docHandler.Delete)

		// File operations
		documents.POST("/upload", auth.RequirePermission(permissionProvider, "documents:write"), docHandler.Upload)
		documents.POST("/:id/upload", auth.RequirePermission(permissionProvider, "documents:write"), docHandler.UploadToExisting)
		documents.GET("/:id/download", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.Download)
		documents.GET("/:id/download-url", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.GetDownloadURL)
		documents.GET("/:id/extraction", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.GetExtraction)
		documents.POST("/:id/extract", auth.RequirePermission(permissionProvider, "documents:write"), docHandler.ReExtract)

		// Versioning
		documents.POST("/:id/versions", auth.RequirePermission(permissionProvider, "documents:write"), docHandler.CreateVersion)

		// Entity-scoped listing
		documents.GET("/entity/:entityType/:entityId", auth.RequirePermission(permissionProvider, "documents:read"), docHandler.ListByEntity)

		// Admin maintenance — rerun the signed-PDF persist pipeline for
		// rows stuck in pending/failed after a webhook subscriber crash
		// or Supabase outage. Gated by documents:write since it both
		// mutates document rows and re-hits Clicksign URLs.
		documents.POST("/backfill-signed", auth.RequirePermission(permissionProvider, "documents:write"), docHandler.BackfillSignedPDFs)
	}

	return r, Services{
		Contract:     contractService,
		Document:     docService,
		Deal:         dealService,
		ContractRepo: contractRepo,
		DealRepo:     dealRepo,
	}
}

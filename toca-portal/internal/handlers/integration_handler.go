package handlers

import (
	"errors"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

type IntegrationHandler struct {
	service *service.IntegrationService
}

func NewIntegrationHandler(service *service.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{service: service}
}

type CreateIntegrationRequest struct {
	Portal          integration.PortalType         `json:"portal" binding:"required,oneof=zap_imoveis olx_grupo_zap vivareal"`
	ContactName     string                         `json:"contact_name" binding:"required"`
	ContactEmail    string                         `json:"contact_email" binding:"required,email"`
	ContactPhone    string                         `json:"contact_phone" binding:"required"`
	SyncFrequency   string                         `json:"sync_frequency" binding:"required,oneof=6h 12h 24h"`
	PublicationType string                         `json:"publication_type" binding:"required,oneof=STANDARD PREMIUM SUPER_PREMIUM"`
	PropertyFilter  integration.FeedPropertyFilter `json:"property_filter"`
}

type UpdateIntegrationRequest struct {
	ContactName     *string                         `json:"contact_name"`
	ContactEmail    *string                         `json:"contact_email"`
	ContactPhone    *string                         `json:"contact_phone"`
	SyncFrequency   *string                         `json:"sync_frequency"`
	PublicationType *string                         `json:"publication_type"`
	PropertyFilter  *integration.FeedPropertyFilter `json:"property_filter"`
	Status          *integration.IntegrationStatus  `json:"status"`
}

// callerTenantID resolve o tenant do chamador a partir do request context
// (pkg/context). Toda rota autenticada de integração passa por aqui — é o tenant
// que vai para o WHERE do repositório (C4).
//
// B10 (revisão de permissões de 2026-09-19): o fallback histórico para
// `account_id` saiu. Em token pessoal (type=individual) ele transformava "sem
// tenant" em "meu próprio id como tenant" — um escopo inventado no cliente para
// um recurso que é da empresa (portal/integração). Sem tenant, 403.
func callerTenantID(c *gin.Context) (string, bool) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil || strings.TrimSpace(tenantID) == "" {
		response.Forbidden(c, "tenant context required")
		return "", false
	}
	return tenantID, true
}

// integrationError responde o erro de forma segura: não encontrado → 404,
// qualquer outra coisa → 500 genérico com o detalhe apenas no log do servidor
// (nunca erro de driver para o cliente).
func integrationError(c *gin.Context, op string, err error) {
	if errors.Is(err, integration.ErrNotFound) {
		response.NotFound(c, "Integration not found")
		return
	}
	log.Printf("[IntegrationHandler] %s failed: %v", op, err)
	response.InternalServerError(c, "failed to process integration request")
}

// Create godoc
// @Summary      Create integration
// @Description  Create a new portal integration (ZAP Imoveis, OLX/Grupo ZAP, VivaReal)
// @Tags         integrations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateIntegrationRequest true "Create Integration Request"
// @Success      201  {object}  response.Response{data=integration.PortalIntegration}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /integrations [post]
func (h *IntegrationHandler) Create(c *gin.Context) {
	var req CreateIntegrationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, ok := callerTenantID(c)
	if !ok {
		return
	}

	integ, err := h.service.Create(
		c.Request.Context(),
		tenantID,
		req.Portal,
		req.ContactName,
		req.ContactEmail,
		req.ContactPhone,
		req.SyncFrequency,
		req.PublicationType,
		req.PropertyFilter,
	)
	if err != nil {
		log.Printf("[IntegrationHandler] Create failed: %v", err)
		response.InternalServerError(c, "failed to create integration")
		return
	}

	response.Created(c, "Integration created successfully", integ)
}

// List godoc
// @Summary      List integrations
// @Description  List all portal integrations for the current tenant
// @Tags         integrations
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]integration.PortalIntegration}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /integrations [get]
func (h *IntegrationHandler) List(c *gin.Context) {
	tenantID, ok := callerTenantID(c)
	if !ok {
		return
	}

	integrations, err := h.service.ListByTenant(c.Request.Context(), tenantID)
	if err != nil {
		integrationError(c, "List", err)
		return
	}

	response.Success(c, "Integrations retrieved successfully", integrations)
}

// GetByID godoc
// @Summary      Get integration by ID
// @Description  Retrieve a single integration by its ID (must belong to the caller's tenant)
// @Tags         integrations
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Integration ID"
// @Success      200  {object}  response.Response{data=integration.PortalIntegration}
// @Failure      404  {object}  response.Response
// @Router       /integrations/{id} [get]
func (h *IntegrationHandler) GetByID(c *gin.Context) {
	tenantID, ok := callerTenantID(c)
	if !ok {
		return
	}

	id := c.Param("id")
	integ, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		integrationError(c, "GetByID", err)
		return
	}
	response.Success(c, "Integration retrieved successfully", integ)
}

// Update godoc
// @Summary      Update integration
// @Description  Update an existing integration's contact info, filters, or status (must belong to the caller's tenant)
// @Tags         integrations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      string                                   true   "Integration ID"
// @Param        request  body      handlers.UpdateIntegrationRequest        true   "Update Integration Request"
// @Success      200      {object}  response.Response{data=integration.PortalIntegration}
// @Failure      400      {object}  response.Response
// @Failure      404      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /integrations/{id} [put]
func (h *IntegrationHandler) Update(c *gin.Context) {
	tenantID, ok := callerTenantID(c)
	if !ok {
		return
	}

	id := c.Param("id")
	var req UpdateIntegrationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// Busca escopada pelo tenant do chamador: integração de outro tenant é
	// 404 e o registro nem é carregado (C4).
	integ, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		integrationError(c, "Update/GetByID", err)
		return
	}

	if req.ContactName != nil {
		integ.ContactName = *req.ContactName
	}
	if req.ContactEmail != nil {
		integ.ContactEmail = *req.ContactEmail
	}
	if req.ContactPhone != nil {
		integ.ContactPhone = *req.ContactPhone
	}
	if req.SyncFrequency != nil {
		integ.SyncFrequency = *req.SyncFrequency
	}
	if req.PublicationType != nil {
		integ.PublicationType = *req.PublicationType
	}
	if req.PropertyFilter != nil {
		integ.PropertyFilter = *req.PropertyFilter
	}
	if req.Status != nil {
		integ.Status = *req.Status
		if *req.Status == integration.IntegrationActive {
			integ.ConsecutiveFails = 0
			integ.ErrorMessage = ""
		}
	}

	if err := h.service.Update(c.Request.Context(), tenantID, integ); err != nil {
		integrationError(c, "Update", err)
		return
	}

	response.Success(c, "Integration updated successfully", integ)
}

// Delete godoc
// @Summary      Delete integration
// @Description  Delete an integration permanently (must belong to the caller's tenant)
// @Tags         integrations
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Integration ID"
// @Success      200  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /integrations/{id} [delete]
func (h *IntegrationHandler) Delete(c *gin.Context) {
	tenantID, ok := callerTenantID(c)
	if !ok {
		return
	}

	id := c.Param("id")
	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		integrationError(c, "Delete", err)
		return
	}
	response.Success(c, "Integration deleted successfully", nil)
}

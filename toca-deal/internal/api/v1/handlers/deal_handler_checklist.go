package handlers

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// CRM checklist endpoints and the built-in template catalog.

// ── Checklist handlers ──

type InitChecklistRequest struct {
	Items []deal.ChecklistItem `json:"items" binding:"required"`
}

type AddChecklistItemRequest struct {
	Name        string                     `json:"name" binding:"required"`
	Category    deal.ChecklistItemCategory `json:"category" binding:"required"`
	PartyID     string                     `json:"party_id,omitempty"`
	Required    bool                       `json:"required"`
	Description string                     `json:"description"`
	// RequestMode: "upload" (anexo) ou "form" (apenas preenchimento).
	RequestMode     string                         `json:"request_mode,omitempty"`
	RequestedFields []deal.ChecklistRequestedField `json:"requested_fields,omitempty"`
}

type UpdateChecklistItemStatusRequest struct {
	// P0/A18: allowlist explícita. Antes era só `binding:"required"` e o valor
	// era gravado literalmente (inclusive qualquer string fora do domínio).
	Status          deal.ChecklistItemStatus `json:"status" binding:"required,oneof=PENDING UPLOADED APPROVED REJECTED"`
	RejectionReason string                   `json:"rejection_reason"`
}

// InitChecklist godoc
// @Summary      Initialize checklist
// @Description  Replace the entire checklist (apply a template)
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.InitChecklistRequest true "Init Checklist Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// requireChecklistApprovalAuthority exige `documents:validate` quando o payload
// tenta estabelecer STATUS DE REVISÃO (aprovar/rejeitar) ou `reviewed_at` na
// checklist.
//
// P0/A18 (reauditoria de 2026-09-20): o A18 fechou a autoaprovação no PATCH, mas
// o PUT `/deals/:id/checklist` — que substitui a checklist inteira — continuava
// aceitando o `ChecklistItem` completo, e o domínio copia o `status` do cliente
// verbatim (só vazio vira PENDING, `domain/deal/deal.go:1008-1019`). Um
// `corretor` (que tem `deals:write` e NÃO tem `documents:validate`) aplicava a
// checklist com os itens obrigatórios já `APPROVED`, satisfazia
// `RequiredChecklistComplete()` e destravava o gate de contrato — o mesmo bypass,
// pelo caminho irmão.
//
// O fluxo legítimo não é afetado: o cliente manda apenas
// `{name, category, required, description}` (DealsContext.initChecklist) e o
// status nasce PENDING. Fail-closed, como o PATCH.
func (h *DealHandler) requireChecklistApprovalAuthority(c *gin.Context, items []deal.ChecklistItem) bool {
	needsAuthority := false
	for _, item := range items {
		if item.Status != "" && item.Status != deal.ChecklistPending {
			needsAuthority = true
			break
		}
		if item.ReviewedAt != nil {
			needsAuthority = true
			break
		}
	}
	if !needsAuthority {
		return true
	}
	return h.permissionProvider != nil && auth.HasPermission(c, h.permissionProvider, "documents:validate")
}

// @Router       /deals/{id}/checklist [put]
func (h *DealHandler) InitChecklist(c *gin.Context) {
	id := c.Param("id")
	var req InitChecklistRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	// A guarda vem ANTES de qualquer leitura/escrita: sem ela o payload com
	// status aprovado já teria sido aceito.
	if !h.requireChecklistApprovalAuthority(c, req.Items) {
		response.Forbidden(c, "permission denied: documents:validate is required to set checklist item status")
		return
	}
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	d, err := h.service.InitDealChecklist(c.Request.Context(), tenantID, id, brokerID, req.Items, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Checklist initialized successfully", d)
}

// AddChecklistItem godoc
// @Summary      Add checklist item
// @Description  Add a single item to the deal checklist
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.AddChecklistItemRequest true "Add Checklist Item Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/checklist [post]
func (h *DealHandler) AddChecklistItem(c *gin.Context) {
	id := c.Param("id")
	var req AddChecklistItemRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	d, err := h.service.AddChecklistItem(c.Request.Context(), tenantID, id, brokerID, req.Name, req.Category, req.Required, req.Description, req.PartyID, req.RequestMode, req.RequestedFields, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Checklist item added successfully", d)
}

// RemoveChecklistItem godoc
// @Summary      Remove checklist item
// @Description  Remove an item from the deal checklist
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        itemId path string true "Checklist Item ID"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/checklist/{itemId} [delete]
func (h *DealHandler) RemoveChecklistItem(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	d, err := h.service.RemoveChecklistItem(c.Request.Context(), tenantID, id, brokerID, itemID, userID)
	if err != nil {
		if errors.Is(err, deal.ErrChecklistItemNotFound) {
			response.NotFound(c, "checklist item not found")
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Checklist item removed successfully", d)
}

// UpdateChecklistItemStatus godoc
// @Summary      Update checklist item status
// @Description  Update the status of a checklist item (approve, reject, upload)
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        itemId path string true "Checklist Item ID"
// @Param        request body handlers.UpdateChecklistItemStatusRequest true "Update Status Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/checklist/{itemId} [patch]
func (h *DealHandler) UpdateChecklistItemStatus(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")
	var req UpdateChecklistItemStatusRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	// P0/A18: aprovar/rejeitar documento é uma capacidade própria
	// (`documents:validate`, matriz: admin/gestor/operacoes). Antes a rota
	// exigia apenas `deals:write`, então um corretor aprovava os próprios
	// documentos e destravava as transições ANALYSIS/CONTRACT protegidas pelo
	// checklist gate. Falha fechado de propósito: sem provider configurado,
	// ninguém aprova (o oposto do gate de valor de deal, que é fail-open — ver
	// achado A23).
	if req.Status == deal.ChecklistApproved || req.Status == deal.ChecklistRejected {
		if h.permissionProvider == nil || !auth.HasPermission(c, h.permissionProvider, "documents:validate") {
			response.Forbidden(c, "permission denied: documents:validate is required to approve or reject checklist items")
			return
		}
	}
	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	d, err := h.service.UpdateChecklistItemStatus(c.Request.Context(), tenantID, id, brokerID, itemID, req.Status, req.RejectionReason, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Checklist item status updated successfully", d)
}

// SolicitChecklistItem godoc
// @Summary      Solicit checklist item
// @Description  Mark a checklist item as solicited (requested from client)
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        itemId path string true "Checklist Item ID"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/checklist/{itemId}/solicit [post]
func (h *DealHandler) SolicitChecklistItem(c *gin.Context) {
	id := c.Param("id")
	itemID := c.Param("itemId")
	var req struct {
		PartyID         string                         `json:"party_id,omitempty"`
		RequestedFields []deal.ChecklistRequestedField `json:"requested_fields,omitempty"`
		RequestMode     string                         `json:"request_mode,omitempty"`
	}
	if !validation.BindJSONOptional(c, &req) {
		return
	}
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	d, err := h.service.SolicitChecklistItem(c.Request.Context(), tenantID, id, brokerID, itemID, req.PartyID, req.RequestedFields, req.RequestMode, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Checklist item solicited successfully", d)
}

// ListChecklistTemplates godoc
// @Summary      List checklist templates available for deals
// @Description  Returns the built-in templates bundled with the platform.
//
//	Per-tenant custom templates ride on top once persistent
//	storage lands (roadmap §4/W4 follow-up). The FE consumes
//	this endpoint with a hardcoded fallback so the experience
//	degrades gracefully if the BE is unreachable.
//
// @Tags         checklist
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]deal.ChecklistTemplate}
// @Router       /checklist-templates [get]
func (h *DealHandler) ListChecklistTemplates(c *gin.Context) {
	response.Success(c, "Checklist templates retrieved", deal.BuiltinChecklistTemplates())
}

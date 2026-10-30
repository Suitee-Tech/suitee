package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-ops/internal/models/maintenance"
	"github.com/oginoino/toca/toca-ops/internal/services"
)

var validPayerRoles = map[string]bool{
	"":       true,
	"tenant": true,
	"owner":  true,
	"shared": true,
}

// validAdvancedBy identifica quem pagou o fornecedor. Vazio assume a
// imobiliária, que é o caso dominante na administração de locação.
var validAdvancedBy = map[string]bool{
	"":             true,
	"imobiliaria":  true,
	"proprietario": true,
	"inquilino":    true,
}

// validBillingTargets define onde o custo entra no financeiro. Vazio mantém o
// destino padrão do payer_role (compatível com chamados antigos).
var validBillingTargets = map[string]bool{
	"":              true,
	"next_charge":   true,
	"next_transfer": true,
	"standalone":    true,
}

type MaintenanceHandler struct {
	service            *services.MaintenanceService
	permissionProvider auth.PermissionProvider
}

func NewMaintenanceHandler(svc *services.MaintenanceService) *MaintenanceHandler {
	return &MaintenanceHandler{service: svc}
}

// WithPermissionProvider habilita o gate de efeito financeiro (achado A12).
func (h *MaintenanceHandler) WithPermissionProvider(p auth.PermissionProvider) *MaintenanceHandler {
	h.permissionProvider = p
	return h
}

// Permissões que autorizam dar destino financeiro ao custo de uma manutenção:
// `charges:write` (cobrança) ou `finance:write` (repasse/dedução). Ambas são de
// admin/financeiro no catálogo — nenhum outro papel move dinheiro por aqui.
const (
	permChargesWrite = "charges:write"
	permFinanceWrite = "finance:write"
)

// createHasBillingEffect informa se o corpo do cadastro introduz política de
// cobrança/reembolso. O financeiro só reage a `payer_role` + custo
// (`maintenance_event_handler.go`), mas `billing_target`/`advanced_by` também
// definem PARA ONDE o dinheiro vai — por isso entram no gate.
func createHasBillingEffect(req CreateMaintenanceRequest) bool {
	return strings.TrimSpace(req.PayerRole) != "" ||
		strings.TrimSpace(req.BillingTarget) != "" ||
		strings.TrimSpace(req.AdvancedBy) != "" ||
		req.EstimatedCost > 0
}

// updateHasBillingEffect informa se a edição mexe em valor ou destino do custo.
func updateHasBillingEffect(req UpdateMaintenanceRequest) bool {
	return strings.TrimSpace(req.PayerRole) != "" ||
		strings.TrimSpace(req.BillingTarget) != "" ||
		strings.TrimSpace(req.AdvancedBy) != "" ||
		req.EstimatedCost > 0 || req.ActualCost > 0
}

// ticketHasBillingPolicy informa se o chamado já carrega custo a lançar: é o que
// faz a conclusão disparar `maintenance.completed` e reconciliar a diferença
// (crédito/desconto) no financeiro.
func ticketHasBillingPolicy(t *maintenance.MaintenanceTicket) bool {
	if t == nil || strings.TrimSpace(t.PayerRole) == "" {
		return false
	}
	return t.EstimatedCost > 0 || t.ActualCost > 0
}

// isCompletedStatus reconhece a conclusão do chamado independentemente de
// acento/caixa ("Concluído" é o valor do domínio; `concluido` aparece em
// payloads legados e no portal). É o gatilho de `maintenance.completed`.
func isCompletedStatus(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	normalized = strings.NewReplacer("í", "i", "á", "a", "é", "e", "õ", "o").Replace(normalized)
	return normalized == "concluido"
}

// requireBillingAuthority exige permissão financeira para o efeito de cobrança.
//
// SECURITY (achado A12 da revisão de permissões de 2026-09-19): criar/editar
// manutenção aceitava `estimated/actual_cost`, `payer_role` e `billing_target` e
// o toca-financial convertia isso em cobrança do inquilino ou dedução do repasse
// do proprietário — sem nenhuma permissão financeira na rota. Quem edita
// chamado (`tickets:write`: admin/gestor/operacoes) não é necessariamente quem
// pode mover dinheiro (admin/financeiro); a partir daqui o efeito financeiro
// exige a permissão financeira, e a negação é fail-closed: sem provider
// configurado ou sem papel no mapa, nega.
func (h *MaintenanceHandler) requireBillingAuthority(c *gin.Context) bool {
	if auth.HasPermission(c, h.permissionProvider, permChargesWrite) ||
		auth.HasPermission(c, h.permissionProvider, permFinanceWrite) {
		return true
	}
	response.Forbidden(c, "billing a maintenance ticket requires a financial permission (charges:write or finance:write)")
	return false
}

type CreateMaintenanceRequest struct {
	TenantID        string   `json:"tenant_id,omitempty"`
	Type            string   `json:"type" binding:"required"`
	TicketType      string   `json:"ticket_type"`
	Category        string   `json:"category" binding:"required"`
	PropertyID      string   `json:"property_id" binding:"required"`
	ContractID      string   `json:"contract_id"`
	RentalID        string   `json:"rental_id"`
	RequesterLeadID string   `json:"requester_lead_id"`
	PropertyTitle   string   `json:"property_title"`
	Requester       string   `json:"requester" binding:"required"`
	Profile         string   `json:"profile" binding:"required"`
	Priority        string   `json:"priority" binding:"required"`
	Description     string   `json:"description"`
	Attachments     []string `json:"attachments"`
	AssignedTo      string   `json:"assigned_to"`
	EstimatedCost   float64  `json:"estimated_cost"`
	PayerRole       string   `json:"payer_role"`
	BillingTarget   string   `json:"billing_target"`
	AdvancedBy      string   `json:"advanced_by"`
	SupplierID      string   `json:"supplier_id"`
	Instructions    string   `json:"instructions"`
	ScheduledAt     string   `json:"scheduled_at"`
}

type UpdateMaintenanceRequest struct {
	Type          string  `json:"type"`
	Category      string  `json:"category"`
	PropertyTitle string  `json:"property_title"`
	Priority      string  `json:"priority"`
	Description   string  `json:"description"`
	AssignedTo    string  `json:"assigned_to"`
	EstimatedCost float64 `json:"estimated_cost"`
	ActualCost    float64 `json:"actual_cost"`
	PayerRole     string  `json:"payer_role"`
	BillingTarget string  `json:"billing_target"`
	AdvancedBy    string  `json:"advanced_by"`
	SupplierID    string  `json:"supplier_id"`
	Instructions  string  `json:"instructions"`
	ScheduledAt   string  `json:"scheduled_at"`
}

type ChangeStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

func (h *MaintenanceHandler) Create(c *gin.Context) {
	var req CreateMaintenanceRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, ok := tenantForRequest(c, req.TenantID)
	if !ok {
		return
	}

	if !validPayerRoles[req.PayerRole] {
		response.ValidationError(c, "payer_role must be one of: tenant, owner, shared")
		return
	}
	if !validBillingTargets[req.BillingTarget] {
		response.ValidationError(c, "billing_target must be one of: next_charge, next_transfer, standalone")
		return
	}
	if !validAdvancedBy[req.AdvancedBy] {
		response.ValidationError(c, "advanced_by must be one of: imobiliaria, proprietario, inquilino")
		return
	}
	if createHasBillingEffect(req) && !h.requireBillingAuthority(c) {
		return
	}

	ticket := maintenance.New(
		req.Type, req.Category, req.PropertyID, req.ContractID,
		req.PropertyTitle, req.Requester, req.Profile, req.Priority,
		req.Description, tenantID,
	)
	if req.TicketType != "" {
		ticket.TicketType = req.TicketType
	} else {
		ticket.TicketType = "customer_request"
	}
	ticket.RentalID = req.RentalID
	ticket.RequesterLeadID = req.RequesterLeadID
	ticket.AssignedTo = req.AssignedTo
	ticket.EstimatedCost = req.EstimatedCost
	ticket.PayerRole = req.PayerRole
	ticket.BillingTarget = req.BillingTarget
	ticket.AdvancedBy = req.AdvancedBy
	ticket.SupplierID = req.SupplierID
	ticket.Instructions = req.Instructions
	if req.ScheduledAt != "" {
		scheduledAt, parseErr := time.Parse(time.RFC3339, req.ScheduledAt)
		if parseErr != nil {
			response.ValidationError(c, "scheduled_at must be RFC3339")
			return
		}
		ticket.ScheduledAt = &scheduledAt
	}
	if len(req.Attachments) > 0 {
		ticket.Attachments = req.Attachments
	}

	if err := h.service.Create(c.Request.Context(), ticket); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Maintenance ticket created successfully", ticket)
}

func (h *MaintenanceHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		tenantID = c.Query("tenant_id")
	}
	if tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}
	ticket, err := h.service.GetByIDForOperator(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Maintenance ticket retrieved successfully", ticket)
}

func (h *MaintenanceHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		tenantID = c.Query("tenant_id")
	}
	if tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateMaintenanceRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if updateHasBillingEffect(req) && !h.requireBillingAuthority(c) {
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if req.Type != "" {
		existing.Type = req.Type
	}
	if req.Category != "" {
		existing.Category = req.Category
	}
	if req.PropertyTitle != "" {
		existing.PropertyTitle = req.PropertyTitle
	}
	if req.Priority != "" {
		existing.Priority = req.Priority
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.AssignedTo != "" {
		existing.AssignedTo = req.AssignedTo
	}
	if req.EstimatedCost > 0 {
		existing.EstimatedCost = req.EstimatedCost
	}
	if req.ActualCost > 0 {
		existing.ActualCost = req.ActualCost
	}
	if req.PayerRole != "" {
		if !validPayerRoles[req.PayerRole] {
			response.ValidationError(c, "payer_role must be one of: tenant, owner, shared")
			return
		}
		existing.PayerRole = req.PayerRole
	}
	if req.BillingTarget != "" {
		if !validBillingTargets[req.BillingTarget] {
			response.ValidationError(c, "billing_target must be one of: next_charge, next_transfer, standalone")
			return
		}
		existing.BillingTarget = req.BillingTarget
	}
	if req.AdvancedBy != "" {
		if !validAdvancedBy[req.AdvancedBy] {
			response.ValidationError(c, "advanced_by must be one of: imobiliaria, proprietario, inquilino")
			return
		}
		existing.AdvancedBy = req.AdvancedBy
	}
	if req.SupplierID != "" {
		existing.SupplierID = req.SupplierID
	}
	if req.Instructions != "" {
		existing.Instructions = req.Instructions
	}
	if req.ScheduledAt != "" {
		scheduledAt, parseErr := time.Parse(time.RFC3339, req.ScheduledAt)
		if parseErr != nil {
			response.ValidationError(c, "scheduled_at must be RFC3339")
			return
		}
		existing.ScheduledAt = &scheduledAt
	}

	if err := h.service.Update(c.Request.Context(), existing); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Maintenance ticket updated successfully", existing)
}

func (h *MaintenanceHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		tenantID = c.Query("tenant_id")
	}
	if tenantID == "" {
		// Sem tenant não há escrita a fazer: o serviço/DB escopam por tenant
		// (P2/item 36).
		response.Forbidden(c, "tenant context required")
		return
	}

	if _, err := h.service.GetByID(c.Request.Context(), tenantID, id); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Maintenance ticket deleted successfully", nil)
}

func (h *MaintenanceHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	status := c.Query("status")
	requesterLeadID := c.Query("requester_lead_id")
	rentalID := c.Query("rental_id")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		tenantID = c.Query("tenant_id")
	}
	if tenantID == "" {
		tenantID = ""
	}

	tickets, total, err := h.service.ListForOperator(c.Request.Context(), tenantID, status, requesterLeadID, rentalID, limit, offset)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Maintenance tickets retrieved successfully", []*maintenance.MaintenanceTicket{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Maintenance tickets retrieved successfully", tickets, page, int(limit), total)
}

func (h *MaintenanceHandler) ListByProperty(c *gin.Context) {
	propertyID := c.Param("id")
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}

	tickets, _, err := h.service.ListByProperty(c.Request.Context(), tenantID, propertyID, 100, 0)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Maintenance tickets retrieved successfully", []*maintenance.MaintenanceTicket{})
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Maintenance tickets retrieved successfully", tickets)
}

func (h *MaintenanceHandler) ChangeStatus(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req ChangeStatusRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	// Concluir um chamado com custo é o gatilho de `maintenance.completed`: o
	// financeiro reconcilia o valor real contra o estimado (crédito ao
	// inquilino ou desconto no repasse). Mudar de status nesse caso também
	// exige permissão financeira (achado A12).
	if isCompletedStatus(req.Status) && ticketHasBillingPolicy(existing) && !h.requireBillingAuthority(c) {
		return
	}

	ticket, err := h.service.ChangeStatus(c.Request.Context(), tenantID, id, req.Status)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	response.Success(c, "Maintenance ticket status updated successfully", ticket)
}

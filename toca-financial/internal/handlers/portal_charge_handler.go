package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// PortalChargeHandler expõe /client-portal/charges — endpoint que
// inquilinos e proprietários (functional_role=cliente) usam para
// consultar charges (cobranças e mensalidades).
//
// Resolução de propriedade (LGPD guard):
//  1. Lê tenant_id + account_id do JWT.
//  2. CrossTenantRepository.LeadIDsForAccount() devolve os lead_ids
//     vinculados ao account dentro do tenant.
//  3. ListByLeadIDs filtra charges onde (lead_id ∈ leads) ou
//     (owner_id ∈ leads) — inquilino vê cobranças que paga, owner
//     vê charges dos imóveis que recebe repasse.
//  4. Sem leads vinculados → lista vazia (não vaza nada).
//
// O endpoint admin `/charges/*` continua existindo (gated por
// `finance:read` + RejectFunctionalRole("cliente")); este handler é
// **adicional** e só serve o portal.
type PortalChargeHandler struct {
	chargeService *service.ChargeService
	crossRepo     repository.CrossTenantRepository
}

func NewPortalChargeHandler(chargeService *service.ChargeService, crossRepo repository.CrossTenantRepository) *PortalChargeHandler {
	return &PortalChargeHandler{
		chargeService: chargeService,
		crossRepo:     crossRepo,
	}
}

// ListMine handles GET /api/v1/client-portal/charges.
//
// @Summary      [Portal] List my charges
// @Description  Cliente lista charges próprias — tenant_lead (cobranças a pagar) ou owner_lead (charges das próprias propriedades).
// @Tags         client-portal
// @Produce      json
// @Security     BearerAuth
// @Param        status query string false "Filter by status (PENDING, PAID, OVERDUE, CANCELLED)"
// @Param        reference_month query string false "Filter by reference month (YYYY-MM)"
// @Param        limit query int false "Page size (default 20, max 100)"
// @Param        offset query int false "Page offset (default 0)"
// @Success      200  {object}  response.Response{data=[]charge.Charge}
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/charges [get]
func (h *PortalChargeHandler) ListMine(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	accountID, _ := context.GetAccountID(c.Request.Context())
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	leadIDs, err := h.crossRepo.LeadIDsForAccount(c.Request.Context(), tenantID, accountID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if len(leadIDs) == 0 {
		response.Paginated(c, "Charges retrieved successfully",
			[]*charge.Charge{}, 1, 20, 0)
		return
	}

	status := c.Query("status")
	referenceMonth := c.Query("reference_month")
	limit := int64(0)
	offset := int64(0)
	if v := c.Query("limit"); v != "" {
		if l, parseErr := strconv.ParseInt(v, 10, 64); parseErr == nil {
			limit = l
		}
	}
	if v := c.Query("offset"); v != "" {
		if o, parseErr := strconv.ParseInt(v, 10, 64); parseErr == nil {
			offset = o
		}
	}

	charges, total, err := h.chargeService.ListByLeadIDs(c.Request.Context(), tenantID, leadIDs, status, referenceMonth, limit, offset)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	pageSize := limit
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(offset/pageSize) + 1
	response.Paginated(c, "Charges retrieved successfully", charges, page, int(pageSize), total)
}

// GetMine handles GET /api/v1/client-portal/charges/:id.
//
// Devolve uma charge específica apenas se algum dos lead_ids do caller
// aparece em `lead_id` ou `owner_id` da charge. Mismatch retorna 404
// (não 403) para não vazar a existência.
//
// @Summary      [Portal] Get my charge by ID
// @Description  Cliente busca uma charge própria (validada por account_id)
// @Tags         client-portal
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Success      200  {object}  response.Response{data=charge.Charge}
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/charges/{id} [get]
func (h *PortalChargeHandler) GetMine(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	accountID, _ := context.GetAccountID(c.Request.Context())
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	leadIDs, err := h.crossRepo.LeadIDsForAccount(c.Request.Context(), tenantID, accountID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	ch, err := h.chargeService.GetByID(c.Request.Context(), tenantID, id)
	if err != nil || ch == nil {
		response.NotFound(c, "charge not found")
		return
	}

	if !canOwnerAccessCharge(ch, leadIDs) {
		response.NotFound(c, "charge not found")
		return
	}

	response.Success(c, "Charge retrieved successfully", ch)
}

// canOwnerAccessCharge: true se algum leadID do caller aparece em
// lead_id ou owner_id da charge. Função pura para facilitar testes.
func canOwnerAccessCharge(ch *charge.Charge, accountLeadIDs []string) bool {
	if ch == nil {
		return false
	}
	for _, id := range accountLeadIDs {
		if id == "" {
			continue
		}
		if ch.LeadID == id || ch.OwnerID == id {
			return true
		}
	}
	return false
}

// resolveChargeForPortal é o helper compartilhado entre GetMine
// e GetReceiptMine — devolve a charge (ou nil + status) já validada
// pela cadeia tenant_id → account_id → lead_ids → ownership da charge.
// Centraliza o LGPD guard: qualquer 404 sai por aqui.
func (h *PortalChargeHandler) resolveChargeForPortal(c *gin.Context) *charge.Charge {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return nil
	}
	accountID, _ := context.GetAccountID(ctx)
	if accountID == "" {
		response.Forbidden(c, "account context required")
		return nil
	}
	leadIDs, err := h.crossRepo.LeadIDsForAccount(ctx, tenantID, accountID)
	if err != nil {
		respondInternalError(c, err)
		return nil
	}
	if len(leadIDs) == 0 {
		response.NotFound(c, "charge not found")
		return nil
	}
	ch, err := h.chargeService.GetByID(ctx, tenantID, c.Param("id"))
	if err != nil || ch == nil {
		response.NotFound(c, "charge not found")
		return nil
	}
	if !canOwnerAccessCharge(ch, leadIDs) {
		response.NotFound(c, "charge not found")
		return nil
	}
	return ch
}

// GetReceiptMine godoc
// @Summary      [Portal] Recibo PDF da charge paga
// @Tags         client-portal
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Success      200 {file} binary
// @Router       /client-portal/charges/{id}/receipt [get]
func (h *PortalChargeHandler) GetReceiptMine(c *gin.Context) {
	ch := h.resolveChargeForPortal(c)
	if ch == nil {
		return
	}
	if ch.Status != charge.StatusPaid {
		response.ValidationError(c, "only PAID charges have a receipt")
		return
	}
	pdf, err := service.RenderChargeReceiptPDF(service.ChargeReceiptContext{Charge: ch})
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.Header("Content-Disposition", "inline; filename=\"recibo-"+ch.ID+".pdf\"")
	c.Data(200, "application/pdf", pdf)
}

// strconv kept imported via go's removeUnused detection — already referenced above.
var _ = strconv.Itoa

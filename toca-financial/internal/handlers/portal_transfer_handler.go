package handlers

import (
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// PortalTransferHandler exposes /client-portal/transfers/* — endpoints
// que owners (functional_role=cliente) usam para consultar e baixar
// comprovantes dos próprios repasses.
//
// Resolução de propriedade (security guard contra LGPD leak):
//  1. Lê tenant_id + account_id do JWT.
//  2. Resolve via CrossTenantRepository.LeadIDsForAccount() os lead_ids
//     pertencentes a esse account dentro do tenant.
//  3. Antes de retornar qualquer transfer/receipt, valida que
//     transfer.OwnerID está dentro desse conjunto de leads.
//  4. Mismatch → 404 (não vazar existência de transfer de outro owner).
//
// O endpoint admin `/transfers/*` continua existindo (gated por
// `finance:read` + RejectFunctionalRole("cliente")) — esse handler é
// **adicional** e só serve o portal.
type PortalTransferHandler struct {
	transferService *service.TransferService
	crossRepo       repository.CrossTenantRepository
	dealClient      *clients.DealClient
	crmClient       *clients.CrmClient
}

func NewPortalTransferHandler(transferService *service.TransferService, crossRepo repository.CrossTenantRepository) *PortalTransferHandler {
	return &PortalTransferHandler{
		transferService: transferService,
		crossRepo:       crossRepo,
	}
}

// SetDealClient é opcional — habilita enrichment do PDF de receipt com
// dados de contract (cliente + título do imóvel).
func (h *PortalTransferHandler) SetDealClient(c *clients.DealClient) {
	h.dealClient = c
}

// SetCrmClient é opcional — habilita enrichment do PDF de receipt com
// nome + documento do owner. Sem ele o PDF degrada para IDs.
func (h *PortalTransferHandler) SetCrmClient(c *clients.CrmClient) {
	h.crmClient = c
}

// canOwnerAccessTransfer checa se o transfer pertence a um lead do
// account_id do caller. Extraído como função pura para facilitar testes
// — não toca em I/O e é o coração do guard LGPD.
func canOwnerAccessTransfer(t *transfer.OwnerTransfer, accountLeadIDs []string) bool {
	if t == nil || t.OwnerID == "" {
		return false
	}
	return slices.Contains(accountLeadIDs, t.OwnerID)
}

// resolveOwnerScope é o passo comum: obtém account_id da JWT e
// recupera os lead_ids que pertencem a esse account. Devolve uma
// resposta HTTP pronta (401/403/500) quando algo falha — handlers só
// precisam checar o booleano `ok`.
func (h *PortalTransferHandler) resolveOwnerScope(c *gin.Context) (tenantID, accountID string, leadIDs []string, ok bool) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return "", "", nil, false
	}
	accountID, _ = context.GetAccountID(c.Request.Context())
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return "", "", nil, false
	}
	leadIDs, err = h.crossRepo.LeadIDsForAccount(c.Request.Context(), tenantID, accountID)
	if err != nil {
		respondInternalError(c, err)
		return "", "", nil, false
	}
	return tenantID, accountID, leadIDs, true
}

// ListMine handles GET /api/v1/client-portal/transfers.
//
// Devolve a lista de repasses cujos owner_id correspondem aos lead_ids
// vinculados ao account_id do caller (resolução via crossRepo).
// Filtros opcionais: ?status, ?year, ?limit, ?offset.
//
// Quando o account não tem leads vinculados (LeadIDsForAccount vazio),
// devolve lista vazia — sem isso o portal aparece "Sem repasses" mesmo
// que o owner ainda esteja sendo configurado pelo admin (transient
// state legítimo).
//
// @Summary      [Portal] List my transfers
// @Description  Owner lista repasses próprios (filtrado por account_id)
// @Tags         client-portal
// @Produce      json
// @Security     BearerAuth
// @Param        status query string false "Filter by status (PENDING, APPROVED, PROCESSED)"
// @Param        year query int false "Filter by reference year (YYYY)"
// @Param        limit query int false "Page size (default 20, max 100)"
// @Param        offset query int false "Page offset (default 0)"
// @Success      200  {object}  response.Response{data=[]transfer.OwnerTransfer}
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/transfers [get]
func (h *PortalTransferHandler) ListMine(c *gin.Context) {
	tenantID, _, leadIDs, ok := h.resolveOwnerScope(c)
	if !ok {
		return
	}
	if len(leadIDs) == 0 {
		response.Paginated(c, "Transfers retrieved successfully",
			[]*transfer.OwnerTransfer{}, 1, 20, 0)
		return
	}

	filter := transfer.PortalListFilter{
		Status: c.Query("status"),
	}
	if yearStr := c.Query("year"); yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil {
			filter.Year = y
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.ParseInt(limitStr, 10, 64); err == nil {
			filter.Limit = l
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.ParseInt(offsetStr, 10, 64); err == nil {
			filter.Offset = o
		}
	}

	transfers, total, err := h.transferService.ListByOwnerLeadIDs(c.Request.Context(), tenantID, leadIDs, filter)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	pageSize := filter.Limit
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(filter.Offset/pageSize) + 1
	response.Paginated(c, "Transfers retrieved successfully", transfers, page, int(pageSize), total)
}

// GetMine handles GET /api/v1/client-portal/transfers/:id.
//
// Retorna o transfer apenas se OwnerID pertence ao account do caller.
// Mismatch (ou transfer inexistente) devolve 404 — não distinguir os
// dois evita confirmar a existência de transfers de terceiros.
//
// @Summary      [Portal] Get my transfer by ID
// @Description  Owner busca um repasse próprio (validado por account_id)
// @Tags         client-portal
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Transfer ID"
// @Success      200  {object}  response.Response{data=transfer.OwnerTransfer}
// @Failure      404  {object}  response.Response
// @Router       /client-portal/transfers/{id} [get]
func (h *PortalTransferHandler) GetMine(c *gin.Context) {
	id := c.Param("id")
	tenantID, _, leadIDs, ok := h.resolveOwnerScope(c)
	if !ok {
		return
	}
	t, err := h.transferService.GetByID(c.Request.Context(), tenantID, id)
	if err != nil || t == nil {
		response.NotFound(c, "transfer not found")
		return
	}
	if !canOwnerAccessTransfer(t, leadIDs) {
		// Não diferenciar de 404 puro — não vazar existência.
		response.NotFound(c, "transfer not found")
		return
	}
	response.Success(c, "Transfer retrieved successfully", t)
}

// ReceiptMine handles GET /api/v1/client-portal/transfers/:id/receipt.
//
// Mesma validação de GetMine — só renderiza o PDF se OwnerID pertence
// ao account do caller. Fix direto de #455/#465 (LGPD privacy leak).
//
// @Summary      [Portal] Download my transfer receipt PDF
// @Description  Owner baixa o comprovante do próprio repasse (validado por account_id)
// @Tags         client-portal
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        id path string true "Transfer ID"
// @Success      200  {file}  binary
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/transfers/{id}/receipt [get]
func (h *PortalTransferHandler) ReceiptMine(c *gin.Context) {
	id := c.Param("id")
	tenantID, _, leadIDs, ok := h.resolveOwnerScope(c)
	if !ok {
		return
	}
	t, err := h.transferService.GetByID(c.Request.Context(), tenantID, id)
	if err != nil || t == nil {
		response.NotFound(c, "transfer not found")
		return
	}
	if !canOwnerAccessTransfer(t, leadIDs) {
		response.NotFound(c, "transfer not found")
		return
	}

	rctx := service.TransferReceiptContext{Transfer: t}
	// Best-effort enrichment (igual ao handler admin) — falhas downstream
	// não devem 500 o receipt do owner; PDF degrada para IDs.
	if h.dealClient != nil && t.ContractID != "" {
		if cdto, derr := h.dealClient.GetContract(c.Request.Context(), tenantID, t.ContractID); derr == nil && cdto != nil {
			rctx.TenantName = cdto.Client
			rctx.PropertyTitle = cdto.PropertyTitle
		}
	}
	if h.crmClient != nil && t.OwnerID != "" {
		if owner, oerr := h.crmClient.GetLead(c.Request.Context(), tenantID, t.OwnerID); oerr == nil && owner != nil {
			rctx.OwnerName = owner.Name
			rctx.OwnerDocument = owner.Documentation
		}
	}

	pdf, err := service.RenderTransferReceiptPDF(rctx)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "inline; filename=\"comprovante-repasse-"+t.ID+".pdf\"")
	c.Data(200, "application/pdf", pdf)
}

package handlers

import (
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-financial/internal/domain/fiscal"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// PortalInformeHandler expõe /client-portal/informes/* — endpoints que
// proprietários (functional_role=cliente) usam para consultar e baixar
// o próprio Informe de Rendimentos, base da declaração de IR (#419/#459).
//
// Mesma proteção LGPD do PortalTransferHandler: o account_id da JWT é
// resolvido para o conjunto de lead_ids do caller (via crossRepo) e só
// informes cujo OwnerID pertence a esse conjunto são devolvidos —
// mismatch (ou informe inexistente) responde 404 para não vazar a
// existência de informes de terceiros.
type PortalInformeHandler struct {
	fiscalService *service.FiscalService
	crossRepo     repository.CrossTenantRepository
}

func NewPortalInformeHandler(fiscalService *service.FiscalService, crossRepo repository.CrossTenantRepository) *PortalInformeHandler {
	return &PortalInformeHandler{fiscalService: fiscalService, crossRepo: crossRepo}
}

// resolveOwnerScope obtém o account_id da JWT e os lead_ids vinculados a
// ele. Devolve uma resposta HTTP pronta (401/403/500) quando algo falha;
// o caller só precisa checar `ok`.
func (h *PortalInformeHandler) resolveOwnerScope(c *gin.Context) (tenantID string, leadIDs []string, ok bool) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return "", nil, false
	}
	accountID, _ := context.GetAccountID(c.Request.Context())
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return "", nil, false
	}
	leadIDs, err = h.crossRepo.LeadIDsForAccount(c.Request.Context(), tenantID, accountID)
	if err != nil {
		respondInternalError(c, err)
		return "", nil, false
	}
	return tenantID, leadIDs, true
}

// ListMine handles GET /api/v1/client-portal/informes.
//
// Devolve os informes de rendimentos cujos owner_id correspondem aos
// lead_ids do account_id do caller. Filtro opcional ?year (YYYY).
//
// @Summary      [Portal] List my income reports
// @Description  Owner lista os próprios Informes de Rendimentos (filtrado por account_id)
// @Tags         client-portal
// @Produce      json
// @Security     BearerAuth
// @Param        year query int false "Filter by year (YYYY)"
// @Success      200  {object}  response.Response{data=[]fiscal.Informe}
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/informes [get]
func (h *PortalInformeHandler) ListMine(c *gin.Context) {
	tenantID, leadIDs, ok := h.resolveOwnerScope(c)
	if !ok {
		return
	}
	if len(leadIDs) == 0 {
		response.Success(c, "Informes retrieved successfully", []*fiscal.Informe{})
		return
	}

	year, _ := strconv.Atoi(c.Query("year"))
	all, err := h.fiscalService.ListInformes(c.Request.Context(), tenantID, year)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	mine := make([]*fiscal.Informe, 0, len(all))
	for _, inf := range all {
		if inf != nil && slices.Contains(leadIDs, inf.OwnerID) {
			mine = append(mine, inf)
		}
	}
	response.Success(c, "Informes retrieved successfully", mine)
}

// DownloadMine handles GET /api/v1/client-portal/informes/:id/pdf.
//
// Renderiza o PDF do informe apenas se o OwnerID pertence ao account do
// caller. Mismatch (ou informe inexistente) → 404.
//
// @Summary      [Portal] Download my income report PDF
// @Description  Owner baixa o PDF do próprio Informe de Rendimentos (validado por account_id)
// @Tags         client-portal
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        id path string true "Informe ID"
// @Success      200  {file}  binary
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/informes/{id}/pdf [get]
func (h *PortalInformeHandler) DownloadMine(c *gin.Context) {
	tenantID, leadIDs, ok := h.resolveOwnerScope(c)
	if !ok {
		return
	}

	informe, err := h.fiscalService.GetInformeByID(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil || informe == nil {
		response.NotFound(c, "informe not found")
		return
	}
	if !slices.Contains(leadIDs, informe.OwnerID) {
		// Não diferenciar de 404 puro — não vazar existência.
		response.NotFound(c, "informe not found")
		return
	}

	pdf, err := service.RenderInformePDF(informe)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.Header("Content-Disposition", "inline; filename=\"informe-rendimentos-"+strconv.Itoa(informe.Year)+".pdf\"")
	c.Data(200, "application/pdf", pdf)
}

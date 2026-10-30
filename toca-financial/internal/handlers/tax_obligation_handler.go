package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-financial/internal/domain/tax_obligation"
	"github.com/oginoino/toca/toca-financial/internal/services"
)

// TaxObligationHandler expõe os tributos retidos a recolher: o passivo que nasce
// quando a imobiliária retém IRRF/INSS/ISS da comissão do corretor e precisa
// recolher a guia até o vencimento.
type TaxObligationHandler struct {
	service *services.TaxObligationService
}

func NewTaxObligationHandler(service *services.TaxObligationService) *TaxObligationHandler {
	return &TaxObligationHandler{service: service}
}

// List godoc
// @Summary  Lista tributos retidos a recolher
// @Tags     financial
// @Router   /financial/tax-obligations [get]
func (h *TaxObligationHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := pkgctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	filter := taxObligationFilterFromQuery(c)
	items, err := h.service.List(ctx, tenantID, filter)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	totals, err := h.service.Totals(ctx, tenantID, filter)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	overdue := 0
	now := time.Now().UTC()
	for _, item := range items {
		if item.Overdue(now) {
			overdue++
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "meta": gin.H{"totals": totals, "overdue_in_page": overdue}})
}

// Get godoc
// @Summary  Detalha uma obrigação de tributo retido
// @Router   /financial/tax-obligations/{id} [get]
func (h *TaxObligationHandler) Get(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	obligation, err := h.service.GetByID(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if obligation == nil {
		response.NotFound(c, "obrigação não encontrada")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": obligation})
}

type settleTaxObligationRequest struct {
	ProofURL      string `json:"proof_url" binding:"required"`
	ProofFilename string `json:"proof_filename"`
	Note          string `json:"note"`
}

// Settle godoc
// @Summary  Dá baixa (recolhimento) numa obrigação de tributo retido
// @Router   /financial/tax-obligations/{id}/settle [post]
func (h *TaxObligationHandler) Settle(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req settleTaxObligationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "informe o comprovante do recolhimento")
		return
	}
	actor, _ := pkgctx.GetAccountID(c.Request.Context())
	obligation, err := h.service.Pay(c.Request.Context(), tenantID, c.Param("id"), actor, req.ProofURL, req.ProofFilename, req.Note)
	if err != nil {
		switch {
		case errors.Is(err, tax_obligation.ErrNotFound):
			response.NotFound(c, "obrigação não encontrada")
		case errors.Is(err, tax_obligation.ErrNotOpen),
			errors.Is(err, tax_obligation.ErrProofRequired),
			errors.Is(err, tax_obligation.ErrInvalidInput),
			errors.Is(err, tax_obligation.ErrAlreadySettled):
			response.UnprocessableEntity(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": obligation})
}

type recomputeTaxObligationsRequest struct {
	CommissionID string `json:"commission_id"`
}

// Recompute godoc
// @Summary  Reconstrói as obrigações a partir dos descontos das comissões
// @Description  Ferramenta de reparo: a obrigação é derivada da comissão, então
//
//	uma falha de sincronização é corrigida reprocessando. Sem `commission_id`,
//	reprocessa as comissões recentes do tenant.
//
// @Router   /financial/tax-obligations/recompute [post]
func (h *TaxObligationHandler) Recompute(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req recomputeTaxObligationsRequest
	_ = c.ShouldBindJSON(&req)
	processed, err := h.service.Recompute(c.Request.Context(), tenantID, req.CommissionID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"commissions_processed": processed}})
}

func taxObligationFilterFromQuery(c *gin.Context) tax_obligation.ListFilter {
	filter := tax_obligation.ListFilter{
		Competence: strings.TrimSpace(c.Query("competence")),
	}
	for _, status := range splitQueryList(c.Query("status")) {
		filter.Status = append(filter.Status, tax_obligation.Status(strings.ToUpper(status)))
	}
	for _, kind := range splitQueryList(c.Query("kind")) {
		filter.Kind = append(filter.Kind, tax_obligation.Kind(strings.ToUpper(kind)))
	}
	if raw := strings.TrimSpace(c.Query("overdue")); raw == "true" || raw == "1" {
		filter.OverdueOnly = true
	}
	if raw := strings.TrimSpace(c.Query("due_before")); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			filter.DueBefore = &parsed
		}
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if limit, err := strconv.Atoi(raw); err == nil {
			filter.Limit = limit
		}
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		if offset, err := strconv.Atoi(raw); err == nil {
			filter.Offset = offset
		}
	}
	return filter
}

// splitQueryList aceita `?status=OPEN,PAID` e `?status=OPEN&status=PAID`.
func splitQueryList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

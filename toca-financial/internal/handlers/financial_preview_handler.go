package handlers

import (
	stdctx "context"
	"errors"
	"fmt"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// FinancialPreviewHandler expõe os endpoints read-only de prévia
// financeira consumidos pela aba "Financeiro" da esteira (antes do
// CONVERTED). Não persiste, não chama o gateway.
type FinancialPreviewHandler struct {
	service financialPreviewReader
}

type financialPreviewReader interface {
	PreviewByDeal(stdctx.Context, string, string) (*service.FinancialPreviewDTO, error)
	// PreviewByDealWithDates aceita as datas do plano pré-contrato (locação).
	PreviewByDealWithDates(stdctx.Context, string, string, *time.Time, *time.Time) (*service.FinancialPreviewDTO, error)
	PreviewByContract(stdctx.Context, string, string) (*service.FinancialPreviewDTO, error)
}

// parsePreviewDateOverrides lê `start_date`/`end_date` (YYYY-MM-DD) da query.
// Ausentes = projeção com as datas do próprio negócio (comportamento anterior).
// Presentes e inválidas = 400, para o operador não ver um calendário silenciosamente
// calculado com outra data.
func parsePreviewDateOverrides(c *gin.Context) (*time.Time, *time.Time, error) {
	parse := func(name string) (*time.Time, error) {
		raw := strings.TrimSpace(c.Query(name))
		if raw == "" {
			return nil, nil
		}
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return nil, fmt.Errorf("%s deve estar no formato AAAA-MM-DD", name)
		}
		return &parsed, nil
	}
	start, err := parse("start_date")
	if err != nil {
		return nil, nil, err
	}
	end, err := parse("end_date")
	if err != nil {
		return nil, nil, err
	}
	if start != nil && end != nil && end.Before(*start) {
		return nil, nil, errors.New("end_date não pode ser anterior a start_date")
	}
	return start, end, nil
}

func NewFinancialPreviewHandler(s financialPreviewReader) *FinancialPreviewHandler {
	return &FinancialPreviewHandler{service: s}
}

// PreviewByDeal godoc
// @Summary      Projeta o ciclo financeiro do deal
// @Description  Retorna charges + commissions + transfers projetados sem persistir nada nem chamar o gateway. Útil para a aba "Financeiro (Prévia)" da esteira antes do contract.signed.
// @Tags         financial-preview
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        start_date query string false "Início do plano pré-contrato (AAAA-MM-DD); ausente usa as datas do negócio"
// @Param        end_date query string false "Término do plano pré-contrato (AAAA-MM-DD); define o residual do último aluguel"
// @Success      200 {object} response.Response{data=service.FinancialPreviewDTO}
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      404 {object} response.Response
// @Router       /financial-preview/deals/{id} [get]
func (h *FinancialPreviewHandler) PreviewByDeal(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	dealID := c.Param("id")
	if dealID == "" {
		response.ValidationError(c, "deal id required")
		return
	}
	// Datas opcionais do PLANO pré-contrato (locação): o negócio ainda não tem
	// início/término, e sem elas a projeção assumiria 30 meses fixos — um
	// calendário diferente do que o operador configurou na esteira.
	startOverride, endOverride, err := parsePreviewDateOverrides(c)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	preview, err := h.service.PreviewByDealWithDates(ctx, tenantID, dealID, startOverride, endOverride)
	if err != nil {
		if errors.Is(err, repositories.ErrHistoricalPartnershipRequired) || errors.Is(err, service.ErrPartnershipFeesReview) {
			response.Conflict(c, "Condições da parceria a revisar: solicite ao responsável financeiro a conferência do acordo aceito e do histórico deste negócio antes de provisionar valores.")
			return
		}
		if isNotFoundFinancialPreviewErr(err) {
			response.NotFound(c, err.Error())
			return
		}
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Financial preview retrieved successfully", preview)
}

// PreviewByContract godoc
// @Summary      Projeta o ciclo financeiro do contrato
// @Description  Como PreviewByDeal, mas usa o contrato como ponto de partida. Útil quando o operador já tem o contrato criado e quer ver a projeção com datas finais.
// @Tags         financial-preview
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Contract ID"
// @Success      200 {object} response.Response{data=service.FinancialPreviewDTO}
// @Failure      403 {object} response.Response
// @Failure      404 {object} response.Response
// @Router       /financial-preview/contracts/{id} [get]
func (h *FinancialPreviewHandler) PreviewByContract(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	contractID := c.Param("id")
	if contractID == "" {
		response.ValidationError(c, "contract id required")
		return
	}
	preview, err := h.service.PreviewByContract(ctx, tenantID, contractID)
	if err != nil {
		if errors.Is(err, repositories.ErrHistoricalPartnershipRequired) || errors.Is(err, service.ErrPartnershipFeesReview) {
			response.Conflict(c, "Condições da parceria a revisar: solicite ao responsável financeiro a conferência do acordo aceito e do histórico deste negócio antes de provisionar valores.")
			return
		}
		if isNotFoundFinancialPreviewErr(err) {
			response.NotFound(c, err.Error())
			return
		}
		respondInternalError(c, err)
		return
	}

	// Audit: register commission distribution data access for tracking
	audit.Record(ctx, audit.Entry{
		Action:     "commission.distribution_loaded",
		EntityType: "Contract",
		EntityID:   contractID,
		After: map[string]any{
			"commissions_count":      len(preview.Commissions),
			"total_commission_cents": preview.TotalCommissionCents,
			"deal_id":                preview.DealID,
			"deal_type":              preview.DealType,
		},
		Metadata: map[string]any{
			"source":      "financial-preview",
			"entity_type": "Contract",
		},
	})

	response.Success(c, "Financial preview retrieved successfully", preview)
}

func isNotFoundFinancialPreviewErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, " not found") || strings.HasSuffix(msg, "not found") {
		return true
	}
	if strings.Contains(msg, "returned status: 404") || strings.Contains(msg, "returned status 404") {
		return true
	}
	return false
}

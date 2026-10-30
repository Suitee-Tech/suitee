package handlers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan_draft"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type SaleBillingPlanHandler struct {
	service    *service.SaleBillingPlanService
	dealClient *clients.DealClient
}

func NewSaleBillingPlanHandler(s *service.SaleBillingPlanService) *SaleBillingPlanHandler {
	return &SaleBillingPlanHandler{service: s}
}

// WithDealClient abre a derivação de owner/property/buyer do negócio
// vinculado quando o wizard não enviou esses campos — a composição
// econômica da venda (repasse + comissões) precisa deles nas cobranças.
func (h *SaleBillingPlanHandler) WithDealClient(c *clients.DealClient) *SaleBillingPlanHandler {
	h.dealClient = c
	return h
}

// CreateSaleBillingPlanRequest is the JSON body of POST
// /sale-billing-plans/contracts/:id. Amount fields are in BRL (float64);
// the handler converts to cents internally so the domain stays integer.
//
// The composite legs (dacao, troca, saldo) are optional. Any leg with
// amount 0 is treated as "not used". When saldo_amount > 0 the operator
// must also provide saldo_due_date (a saldo charge needs a deterministic
// vencimento).
type CreateSaleBillingPlanRequest struct {
	DealID                 string    `json:"deal_id"`
	BuyerPartyID           string    `json:"buyer_party_id"`
	OwnerID                string    `json:"owner_id"`
	PropertyID             string    `json:"property_id"`
	Mode                   string    `json:"mode" binding:"required,oneof=SINGLE INSTALLMENTS"`
	TotalAmount            float64   `json:"total_amount" binding:"required,gt=0"`
	DownPaymentAmount      float64   `json:"down_payment_amount"`
	InstallmentAmount      float64   `json:"installment_amount"`
	InstallmentCount       int       `json:"installment_count" binding:"required,min=1"`
	FirstDueDate           time.Time `json:"first_due_date" binding:"required"`
	RecurrenceIntervalDays int       `json:"recurrence_interval_days"`
	// Composite legs
	DacaoAmount      float64    `json:"dacao_amount,omitempty"`
	DacaoDescription string     `json:"dacao_description,omitempty"`
	TrocaAmount      float64    `json:"troca_amount,omitempty"`
	TrocaDescription string     `json:"troca_description,omitempty"`
	SaldoAmount      float64    `json:"saldo_amount,omitempty"`
	SaldoDueDate     *time.Time `json:"saldo_due_date,omitempty"`
	SaldoDescription string     `json:"saldo_description,omitempty"`
}

func (h *SaleBillingPlanHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	contractID := c.Param("id")
	if contractID == "" {
		response.ValidationError(c, "contract id is required")
		return
	}
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req CreateSaleBillingPlanRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	// Deriva owner/property/buyer do negócio vinculado quando o wizard
	// enviou apenas deal_id (composição econômica da venda precisa das
	// três identidades para congelar repasse + comissões nas cobranças).
	if strings.TrimSpace(req.DealID) != "" && h.dealClient != nil &&
		(strings.TrimSpace(req.OwnerID) == "" || strings.TrimSpace(req.PropertyID) == "" || strings.TrimSpace(req.BuyerPartyID) == "") {
		deal, dealErr := h.dealClient.GetDeal(ctx, tenantID, req.DealID)
		if dealErr != nil {
			response.ValidationError(c, fmt.Sprintf("deal lookup failed: %v", dealErr))
			return
		}
		if deal != nil {
			if strings.TrimSpace(req.OwnerID) == "" {
				req.OwnerID = deal.OwnerID
			}
			if strings.TrimSpace(req.PropertyID) == "" {
				req.PropertyID = deal.PropertyID
			}
			if strings.TrimSpace(req.BuyerPartyID) == "" {
				req.BuyerPartyID = deal.CustomerID
			}
		}
	}
	composite := sale_billing_plan.CompositeInput{
		DacaoAmountCents: int64(math.Round(req.DacaoAmount * 100)),
		DacaoDescription: req.DacaoDescription,
		TrocaAmountCents: int64(math.Round(req.TrocaAmount * 100)),
		TrocaDescription: req.TrocaDescription,
		SaldoAmountCents: int64(math.Round(req.SaldoAmount * 100)),
		SaldoDueDate:     req.SaldoDueDate,
		SaldoDescription: req.SaldoDescription,
	}
	plan, created, err := h.service.CreateAndGenerateCharges(
		ctx, tenantID, contractID, req.DealID, req.BuyerPartyID, req.OwnerID, req.PropertyID,
		sale_billing_plan.Mode(req.Mode),
		int64(math.Round(req.TotalAmount*100)),
		int64(math.Round(req.DownPaymentAmount*100)),
		int64(math.Round(req.InstallmentAmount*100)),
		req.InstallmentCount, req.FirstDueDate, req.RecurrenceIntervalDays,
		composite,
	)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":            plan,
		"charges_created": created,
	})
}

func (h *SaleBillingPlanHandler) GetByContract(c *gin.Context) {
	ctx := c.Request.Context()
	contractID := c.Param("id")
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	plan, err := h.service.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if plan == nil {
		response.NotFound(c, "sale billing plan not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": plan})
}

// UpsertDraftRequest é o JSON body do POST
// /sale-billing-plans/drafts/deals/:id. Mesmos campos do Create, mas
// sem contract_id (o draft é amarrado ao deal, não ao contract — o
// contract ainda não existe nesse momento).
type UpsertDraftRequest struct {
	Mode                   string     `json:"mode" binding:"required,oneof=SINGLE INSTALLMENTS"`
	TotalAmount            float64    `json:"total_amount" binding:"required,gt=0"`
	DownPaymentAmount      float64    `json:"down_payment_amount"`
	InstallmentAmount      float64    `json:"installment_amount"`
	InstallmentCount       int        `json:"installment_count" binding:"required,min=1"`
	FirstDueDate           time.Time  `json:"first_due_date" binding:"required"`
	RecurrenceIntervalDays int        `json:"recurrence_interval_days"`
	DacaoAmount            float64    `json:"dacao_amount,omitempty"`
	DacaoDescription       string     `json:"dacao_description,omitempty"`
	TrocaAmount            float64    `json:"troca_amount,omitempty"`
	TrocaDescription       string     `json:"troca_description,omitempty"`
	SaldoAmount            float64    `json:"saldo_amount,omitempty"`
	SaldoDueDate           *time.Time `json:"saldo_due_date,omitempty"`
	SaldoDescription       string     `json:"saldo_description,omitempty"`
}

// UpsertDraft godoc
// @Summary      Save or replace the SALE billing plan draft for a deal
// @Description  Operator gravam a forma de pagamento ANTES da assinatura. Idempotente por (tenant, deal). Consumido no contract.signed.
// @Tags         sale-billing-plans
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.UpsertDraftRequest true "Draft payload"
// @Success      200 {object} response.Response{data=sale_billing_plan_draft.Draft}
// @Router       /sale-billing-plans/drafts/deals/{id} [post]
func (h *SaleBillingPlanHandler) UpsertDraft(c *gin.Context) {
	ctx := c.Request.Context()
	dealID := c.Param("id")
	if dealID == "" {
		response.ValidationError(c, "deal id is required")
		return
	}
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req UpsertDraftRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	accountID, _ := context.GetAccountID(ctx)
	composite := sale_billing_plan.CompositeInput{
		DacaoAmountCents: int64(math.Round(req.DacaoAmount * 100)),
		DacaoDescription: req.DacaoDescription,
		TrocaAmountCents: int64(math.Round(req.TrocaAmount * 100)),
		TrocaDescription: req.TrocaDescription,
		SaldoAmountCents: int64(math.Round(req.SaldoAmount * 100)),
		SaldoDueDate:     req.SaldoDueDate,
		SaldoDescription: req.SaldoDescription,
	}
	draft, err := sale_billing_plan_draft.New(
		tenantID, dealID,
		sale_billing_plan.Mode(req.Mode),
		int64(math.Round(req.TotalAmount*100)),
		int64(math.Round(req.DownPaymentAmount*100)),
		int64(math.Round(req.InstallmentAmount*100)),
		req.InstallmentCount, req.FirstDueDate, req.RecurrenceIntervalDays,
		composite, accountID,
	)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	if err := h.service.UpsertDraft(ctx, draft); err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Draft saved successfully", draft)
}

// GetDraft godoc
// @Summary      Get the active SALE billing plan draft for a deal
// @Description  Devolve o draft pré-assinatura ativo. 404 quando ausente — não significa erro, só significa "operador ainda não configurou".
// @Tags         sale-billing-plans
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Success      200 {object} response.Response{data=sale_billing_plan_draft.Draft}
// @Failure      404 {object} response.Response
// @Router       /sale-billing-plans/drafts/deals/{id} [get]
func (h *SaleBillingPlanHandler) GetDraft(c *gin.Context) {
	ctx := c.Request.Context()
	dealID := c.Param("id")
	if dealID == "" {
		response.ValidationError(c, "deal id is required")
		return
	}
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	draft, err := h.service.GetActiveDraft(ctx, tenantID, dealID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if draft == nil {
		response.NotFound(c, "draft not found")
		return
	}
	response.Success(c, "Draft retrieved successfully", draft)
}

// SalePlanReadinessResponse is the wire shape of
// GET /financial/internal/deals/:id/sale-plan-readiness.
//
// T-01: consumido pelo toca-deal antes de gerar/enviar um contrato de venda.
// O contract.signed materializa SÓ o draft aprovado pelo operador (sem
// fabricar fallback à vista), então a única forma de garantir que a venda
// financiada continue cobrando como combinado é recusar contratos de venda
// sem draft ativo — o guard de "plano concluído" deixou de ser só de
// frontend.
type SalePlanReadinessResponse struct {
	Active           bool   `json:"active"`
	TotalAmountCents int64  `json:"total_amount_cents,omitempty"`
	Mode             string `json:"mode,omitempty"`
}

// SalePlanReadiness godoc
// @Summary      Readiness of the SALE payment plan for a deal (internal)
// @Description  Tells the deal service whether an active billing draft exists for the deal and its frozen total. Service keys and user JWTs are accepted; the tenant is resolved from the credential.
// @Tags         financial-internal
// @Produce      json
// @Security     BearerAuth
// @Param        id        path string true "Deal ID"
// @Param        tenant_id query string false "Tenant ID (service callers only; resolved from credential)"
// @Success      200 {object} response.Response{data=handlers.SalePlanReadinessResponse}
// @Failure      400 {object} response.Response
// @Failure      401 {object} response.Response
// @Failure      403 {object} response.Response
// @Router       /financial/internal/deals/{id}/sale-plan-readiness [get]
func (h *SaleBillingPlanHandler) SalePlanReadiness(c *gin.Context) {
	dealID := c.Param("id")
	if dealID == "" {
		response.ValidationError(c, "deal ID is required")
		return
	}

	// SECURITY (achado A3): o tenant vem da CREDENCIAL (JWT do usuário ou
	// chave de serviço com allowlist), nunca do chamador.
	tenantID, err := auth.ResolveTenantForInternalRequest(c, c.Query("tenant_id"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInternalCrossTenant):
			response.Forbidden(c, err.Error())
		case errors.Is(err, auth.ErrInternalTenantRequired):
			response.ValidationError(c, err.Error())
		default:
			response.Unauthorized(c, err.Error())
		}
		return
	}

	// Rota interna: o chamador é o serviço de negócios (chave interna), sem
	// claims de JWT — a leitura precisa do pool de serviço.
	draft, err := h.service.GetActiveDraftForService(c.Request.Context(), tenantID, dealID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if draft == nil {
		response.Success(c, "Sale plan readiness retrieved", SalePlanReadinessResponse{Active: false})
		return
	}
	response.Success(c, "Sale plan readiness retrieved", SalePlanReadinessResponse{
		Active:           true,
		TotalAmountCents: draft.TotalAmountCents,
		Mode:             string(draft.Mode),
	})
}

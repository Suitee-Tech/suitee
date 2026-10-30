package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// Force usage of commission package for Swagger
var _ = commission.Commission{}

type CommissionHandler struct {
	service     *service.CommissionService
	proofPolicy proofPolicySettings
	// permissionProvider avalia a permissão dedicada de aprovação de ajuste de
	// valor (M11: deduções de comissão acima do limiar). Nil ⇒ nega.
	permissionProvider auth.PermissionProvider
}

func NewCommissionHandler(service *service.CommissionService) *CommissionHandler {
	return &CommissionHandler{service: service}
}

// WithPermissionProvider injeta o resolvedor de permissões do gate de ajuste.
func (h *CommissionHandler) WithPermissionProvider(provider auth.PermissionProvider) *CommissionHandler {
	h.permissionProvider = provider
	return h
}

// SetProofPolicy conecta a política de comprovante do tenant para as baixas
// manuais deste handler. Sem isso vale o default do domínio, que exige.
func (h *CommissionHandler) SetProofPolicy(settings proofPolicySettings) {
	h.proofPolicy = settings
}

func respondCommissionLifecycleError(c *gin.Context, err error) {
	if isSegregationOfDutiesError(err) {
		response.Conflict(c, err.Error())
		return
	}
	if errors.Is(err, commission.ErrLinkedPayoutLifecycleManaged) ||
		errors.Is(err, commission.ErrConcurrentTransition) ||
		errors.Is(err, commission.ErrFundingIncomplete) ||
		errors.Is(err, commission.ErrFundingGateConflict) {
		response.Conflict(c, err.Error())
		return
	}
	// service.ErrManualProofRequired (falta o comprovante exigido pela política
	// do tenant) também cai aqui: é entrada incompleta, não falha de servidor.
	response.ValidationError(c, err.Error())
}

func getIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := c.GetHeader("Idempotency-Key"); v != "" {
		return v
	}
	return c.GetHeader("X-Idempotency-Key")
}

// CreateCommissionRequest aceita os campos legados obrigatórios e três
// campos opcionais (role, percentage, contract_id). Quando os opcionais
// estão presentes, delega para CreateWithRole para evitar commissions
// órfãs de role/contract_id. Sem eles, cai no construtor legacy New()
// (compatível com callers antigos que não enviam estes campos).
//
// source_event_id é usado para idempotência: se presente, o handler
// delega para os métodos *FromEvent (CreateFromRequest / CreateWithRoleFromEvent)
// que verificam duplicatas via ON CONFLICT no banco.
type CreateCommissionRequest struct {
	DealID        string  `json:"deal_id" binding:"required"`
	BrokerID      string  `json:"broker_id" binding:"required"`
	Amount        float64 `json:"amount" binding:"required,gt=0"`
	Role          string  `json:"role"`
	Percentage    float64 `json:"percentage"`
	ContractID    string  `json:"contract_id"`
	SourceEventID string  `json:"source_event_id"`
}

// Create godoc
// @Summary      Create commission
// @Description  Create a new commission (legacy path — prefer POST /commissions/batch when splits are available)
// @Tags         commissions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateCommissionRequest true "Create Commission Request"
// @Success      201  {object}  response.Response{data=commission.Commission}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /commissions [post]
func (h *CommissionHandler) Create(c *gin.Context) {
	var req CreateCommissionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	idempotencyKey := getIdempotencyKey(c)
	// source_event_id no corpo da requisição tem prioridade sobre o header
	effectiveSourceID := req.SourceEventID
	if effectiveSourceID == "" && idempotencyKey != "" {
		effectiveSourceID = events.DeriveNotificationID(idempotencyKey, "commission.create",
			req.DealID, req.BrokerID, req.Role,
			strconv.FormatFloat(req.Percentage, 'f', 4, 64),
			req.ContractID, strconv.FormatFloat(req.Amount, 'f', 2, 64))
	}
	var comm *commission.Commission
	// SECURITY (achado A13 da revisão de permissões de 2026-09-19): o solicitante
	// fica gravado na comissão para que a aprovação possa recusar criador =
	// aprovador. Vem do vínculo autenticado (account_id), nunca do corpo.
	actor, _ := context.GetAccountID(c.Request.Context())

	// Quando role, percentage ou contract_id estão presentes, usa
	// CreateWithRole (com todos os campos populados) para evitar
	// commissions órfãs. O caminho legacy (sem esses campos) usa New().
	hasRich := req.Role != "" || req.Percentage > 0 || req.ContractID != ""
	if hasRich {
		if effectiveSourceID != "" {
			comm, err = h.service.CreateWithRoleFromEvent(c.Request.Context(), effectiveSourceID, req.DealID, req.BrokerID, req.ContractID, req.Role, req.Percentage, req.Amount, tenantID, "", "", "", "", actor)
		} else {
			comm, err = h.service.CreateWithRole(c.Request.Context(), req.DealID, req.BrokerID, req.ContractID, req.Role, req.Percentage, req.Amount, tenantID, actor)
		}
	} else {
		if effectiveSourceID != "" {
			comm, err = h.service.CreateFromRequest(c.Request.Context(), effectiveSourceID, req.DealID, req.BrokerID, req.Amount, tenantID, actor)
		} else {
			comm, err = h.service.Create(c.Request.Context(), req.DealID, req.BrokerID, req.Amount, tenantID, actor)
		}
	}
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Created(c, "Commission created successfully", comm)
}

// CommissionSplitInput is one broker's slice in a manual commission posting.
type CommissionSplitInput struct {
	BrokerID   string  `json:"broker_id" binding:"required"`
	Role       string  `json:"role"`
	Percentage float64 `json:"percentage"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
}

type CreateCommissionBatchRequest struct {
	DealID     string                 `json:"deal_id" binding:"required"`
	ContractID string                 `json:"contract_id"`
	Splits     []CommissionSplitInput `json:"splits" binding:"required,min=1,dive"`
}

// CreateBatch godoc
// @Summary      Create commissions in batch (manual broker splits)
// @Description  Post one commission per broker split for a deal in a single call
// @Tags         commissions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateCommissionBatchRequest true "Batch Request"
// @Success      201  {object}  response.Response{data=[]commission.Commission}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /commissions/batch [post]
func (h *CommissionHandler) CreateBatch(c *gin.Context) {
	var req CreateCommissionBatchRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// All splits are validated by binding (dive) before any write. The loop
	// is not transactional; a mid-loop repo failure leaves earlier splits
	// persisted — acceptable for an operator action that can be re-run, and
	// each commission is independently editable/cancellable afterwards.

	// Valida que a soma dos percentages não excede 100% para evitar
	// lançamentos inconsistentes (ex: 3 splits de 50% cada = 150%).
	var sumPct float64
	for _, sp := range req.Splits {
		sumPct += sp.Percentage
	}
	if sumPct > 100.01 { // tolerância de arredondamento float
		response.ValidationError(c, fmt.Sprintf("soma dos percentages (%.1f%%) excede 100%%", sumPct))
		return
	}

	out := make([]*commission.Commission, 0, len(req.Splits))
	idempotencyKey := getIdempotencyKey(c)
	// A13: mesmo registro de solicitante do Create — criador ≠ aprovador.
	actor, _ := context.GetAccountID(c.Request.Context())
	for _, sp := range req.Splits {
		var comm *commission.Commission
		var err error
		if idempotencyKey != "" {
			sourceEventID := events.DeriveNotificationID(
				idempotencyKey,
				"commission.batch",
				req.DealID,
				req.ContractID,
				sp.BrokerID,
				sp.Role,
				strconv.FormatFloat(sp.Percentage, 'f', 4, 64),
				strconv.FormatFloat(sp.Amount, 'f', 2, 64),
			)
			comm, err = h.service.CreateWithRoleFromEvent(c.Request.Context(), sourceEventID, req.DealID, sp.BrokerID, req.ContractID, sp.Role, sp.Percentage, sp.Amount, tenantID, "", "", "", "", actor)
		} else {
			comm, err = h.service.CreateWithRole(c.Request.Context(), req.DealID, sp.BrokerID, req.ContractID, sp.Role, sp.Percentage, sp.Amount, tenantID, actor)
		}
		if err != nil {
			respondInternalError(c, err)
			return
		}
		out = append(out, comm)
	}

	response.Created(c, "Commissions created successfully", out)
}

// List godoc
// @Summary      List commissions
// @Description  List commissions with pagination
// @Tags         commissions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response{data=[]commission.Commission}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /commissions [get]
func (h *CommissionHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "10"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	commissions, total, err := h.service.List(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Commissions retrieved successfully", commissions, page, int(limit), total)
}

// GetByID godoc
// @Summary      Get commission by ID
// @Description  Get a commission by its ID
// @Tags         commissions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Commission ID"
// @Success      200  {object}  response.Response{data=commission.Commission}
// @Failure      404  {object}  response.Response
// @Router       /commissions/{id} [get]
func (h *CommissionHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	comm, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Commission retrieved successfully", comm)
}

// ListByDeal godoc
// @Summary      List commissions by deal
// @Tags         commissions
// @Produce      json
// @Security     BearerAuth
// @Param        dealId path string true "Deal ID"
// @Success      200 {object} response.Response{data=[]commission.Commission}
// @Router       /commissions/by-deal/{dealId} [get]
func (h *CommissionHandler) ListByDeal(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	out, err := h.service.ListByDeal(c.Request.Context(), tenantID, c.Param("dealId"))
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Commissions retrieved successfully", out)
}

// ListByBroker godoc
// @Summary      List commissions by broker
// @Tags         commissions
// @Produce      json
// @Security     BearerAuth
// @Param        brokerId path string true "Broker ID"
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200 {object} response.Response{data=[]commission.Commission}
// @Router       /commissions/by-broker/{brokerId} [get]
func (h *CommissionHandler) ListByBroker(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	out, total, err := h.service.ListByBroker(c.Request.Context(), tenantID, c.Param("brokerId"), limit, offset)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	page := int((offset / limit) + 1)
	response.Paginated(c, "Commissions retrieved successfully", out, page, int(limit), total)
}

// Approve godoc
// @Summary      Approve commission (PENDING → APPROVED)
// @Tags         commissions
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Commission ID"
// @Success      200 {object} response.Response{data=commission.Commission}
// @Router       /commissions/{id}/approve [post]
func (h *CommissionHandler) ApproveCommission(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	actor, _ := context.GetAccountID(ctx)
	// SECURITY (achado A13 da revisão de permissões de 2026-09-19): criador ≠
	// aprovador. A dispensa para quem responde pela operação financeira sai do
	// papel autenticado (admin/gestor), nunca do corpo da requisição.
	ctx = withFinanceAuthorityWaiver(ctx)
	updated, err := h.service.Approve(ctx, tenantID, c.Param("id"), actor)
	if err != nil {
		respondCommissionLifecycleError(c, err)
		return
	}
	response.Success(c, "Commission approved successfully", updated)
}

type CommissionPayRequest struct {
	ProofURL string `json:"proof_url"`
	Note     string `json:"note"`
}

// MarkPaid godoc
// @Summary      Mark commission as paid (PENDING/APPROVED → PAID)
// @Tags         commissions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Commission ID"
// @Param        request body handlers.CommissionPayRequest true "Pay Request"
// @Success      200 {object} response.Response{data=commission.Commission}
// @Router       /commissions/{id}/pay [post]
func (h *CommissionHandler) MarkPaid(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req CommissionPayRequest
	_ = c.ShouldBindJSON(&req)
	// Confirmação manual do operador: a política do tenant é aplicada na borda
	// HTTP, porque qualquer cliente pode chamar o endpoint — a interface não é
	// barreira suficiente.
	if err := requireManualProof(ctx, h.proofPolicy, tenantID, req.ProofURL); err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	actor, _ := context.GetAccountID(ctx)
	// SECURITY (achado A13 da revisão de permissões de 2026-09-19): aprovador ≠
	// pagador. Comissão ligada a payout order continua sob a segregação do
	// payout, que é o aggregate root dela.
	ctx = withFinanceAuthorityWaiver(ctx)
	updated, err := h.service.MarkPaidWithProof(ctx, tenantID, c.Param("id"), req.ProofURL, req.Note, actor)
	if err != nil {
		respondCommissionLifecycleError(c, err)
		return
	}
	response.Success(c, "Commission marked as paid successfully", updated)
}

type CommissionCancelRequest struct {
	Reason string `json:"reason"`
}

// Cancel godoc
// @Summary      Cancel commission
// @Tags         commissions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Commission ID"
// @Param        request body handlers.CommissionCancelRequest false "Cancel Request"
// @Success      200 {object} response.Response{data=commission.Commission}
// @Router       /commissions/{id}/cancel [post]
func (h *CommissionHandler) CancelCommission(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req CommissionCancelRequest
	_ = c.ShouldBindJSON(&req)
	updated, err := h.service.Cancel(c.Request.Context(), tenantID, c.Param("id"), req.Reason)
	if err != nil {
		respondCommissionLifecycleError(c, err)
		return
	}
	response.Success(c, "Commission cancelled successfully", updated)
}

// ReverseCommission estorna uma comissão (APPROVED/PAID → REVERSED) via
// CommissionService.Reverse — o lifecycle de estorno (clawback) do domínio.
func (h *CommissionHandler) ReverseCommission(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req CommissionCancelRequest
	_ = c.ShouldBindJSON(&req)
	updated, err := h.service.Reverse(c.Request.Context(), tenantID, c.Param("id"), req.Reason)
	if err != nil {
		respondCommissionLifecycleError(c, err)
		return
	}
	response.Success(c, "Commission reversed successfully", updated)
}

type AddDeductionRequest struct {
	DeductionID string  `json:"deduction_id"`
	Kind        string  `json:"kind" binding:"required"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount" binding:"gt=0"`
	// RatePct/BaseCents identificam o lançamento POR ALÍQUOTA (INSS de autônomo,
	// ISS municipal): sem estes campos aqui o binding os descartava e o desconto
	// era gravado sem a memória de cálculo, mesmo com o domínio já validando.
	RatePct   float64 `json:"rate_pct"`
	BaseCents int64   `json:"base_cents"`
}

// AddDeduction godoc
// @Summary      Add a deduction to a commission
// @Tags         commissions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Commission ID"
// @Param        request body handlers.AddDeductionRequest true "Deduction"
// @Success      200 {object} response.Response{data=commission.Commission}
// @Router       /commissions/{id}/deductions [post]
func (h *CommissionHandler) AddDeduction(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req AddDeductionRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	// M11 — dedução de comissão é ajuste de valor: acima do limiar exige a
	// permissão dedicada de aprovação, não a mesma permissão de escrita.
	threshold := adjustmentApprovalThresholdBRL()
	requiresApproval := adjustmentRequiresApproval(req.Amount, threshold)
	approved := false
	if requiresApproval {
		if !actorCanApproveAdjustment(c, h.permissionProvider) {
			audit.Record(c.Request.Context(), audit.Entry{
				Action:     "finance.commission_deduction_denied",
				EntityType: "commission",
				EntityID:   c.Param("id"),
				After:      map[string]any{"deduction_amount": req.Amount, "kind": req.Kind},
				Metadata: map[string]any{
					"tenant_id":           tenantID,
					"magnitude_brl":       req.Amount,
					"threshold_brl":       threshold,
					"required_permission": AdjustmentApprovalPermission,
				},
			})
			response.Error(c, http.StatusForbidden, fmt.Sprintf(
				"dedução de %s excede o limiar de %s e exige a permissão %s",
				money.BRL(req.Amount), money.BRL(threshold), AdjustmentApprovalPermission))
			return
		}
		approved = true
	}

	deductionIdentity := strings.TrimSpace(req.DeductionID)
	if deductionIdentity == "" {
		deductionIdentity = strings.TrimSpace(getIdempotencyKey(c))
	}
	deductionID := ""
	if deductionIdentity != "" {
		deductionID = events.DeriveNotificationID(
			"commission_deduction", tenantID, c.Param("id"), deductionIdentity,
		)
	}
	updated, err := h.service.AddDeduction(c.Request.Context(), tenantID, c.Param("id"), commission.Deduction{
		ID:          deductionID,
		Kind:        commission.DeductionKind(req.Kind),
		Description: req.Description,
		Amount:      req.Amount,
		RatePct:     req.RatePct,
		BaseCents:   req.BaseCents,
	})
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	if requiresApproval && approved {
		audit.Record(c.Request.Context(), audit.Entry{
			Action:     "finance.commission_deduction_approved",
			EntityType: "commission",
			EntityID:   c.Param("id"),
			After:      map[string]any{"deduction_amount": req.Amount, "kind": req.Kind},
			Metadata: map[string]any{
				"tenant_id":     tenantID,
				"magnitude_brl": req.Amount,
				"threshold_brl": threshold,
			},
		})
	}
	response.Success(c, "Deduction added successfully", updated)
}

// RemoveDeduction godoc
// @Summary      Remove a deduction by stable ID
// @Tags         commissions
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Commission ID"
// @Param        deductionId path string true "Deduction ID"
// @Success      200 {object} response.Response{data=commission.Commission}
// @Router       /commissions/{id}/deductions/{deductionId} [delete]
func (h *CommissionHandler) RemoveDeduction(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	deductionID := strings.TrimSpace(c.Param("deductionId"))
	if deductionID == "" {
		response.ValidationError(c, "deduction id is required")
		return
	}
	updated, err := h.service.RemoveDeduction(c.Request.Context(), tenantID, c.Param("id"), deductionID)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Deduction removed successfully", updated)
}

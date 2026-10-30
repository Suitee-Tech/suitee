package handlers

import (
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

func respondPayoutOrderLifecycleError(c *gin.Context, err error) {
	if isSegregationOfDutiesError(err) {
		response.Conflict(c, err.Error())
		return
	}
	if errors.Is(err, payout_order.ErrFundingIncomplete) || errors.Is(err, payout_order.ErrFundingGateConflict) || errors.Is(err, payout_order.ErrConcurrentTransition) {
		response.Conflict(c, err.Error())
		return
	}
	response.ValidationError(c, err.Error())
}

type PayoutOrderHandler struct {
	service       *service.PayoutOrderService
	orchestration *service.PayoutOrchestrationService
	proofPolicy   proofPolicySettings
}

func NewPayoutOrderHandler(s *service.PayoutOrderService, orch *service.PayoutOrchestrationService) *PayoutOrderHandler {
	return &PayoutOrderHandler{service: s, orchestration: orch}
}

// SetProofPolicy conecta a política de comprovante do tenant. A exigência era
// incondicional no domínio, então o setting payout_proof_required não governava
// nada: desligá-lo não tinha efeito. Agora é ele quem decide.
func (h *PayoutOrderHandler) SetProofPolicy(settings proofPolicySettings) {
	h.proofPolicy = settings
}

func getPayoutOrderIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := c.GetHeader("Idempotency-Key"); v != "" {
		return v
	}
	return c.GetHeader("X-Idempotency-Key")
}

// List returns payout_orders for the caller's tenant. Filters are optional
// query params: ?status=PENDING_REVIEW&recipient_id=...&group_id=...
func (h *PayoutOrderHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	filter := payout_order.ListFilter{
		Status:        c.Query("status"),
		SourceType:    c.Query("source_type"),
		SourceID:      c.Query("source_id"),
		ContractID:    c.Query("contract_id"),
		RecipientType: c.Query("recipient_type"),
		RecipientID:   c.Query("recipient_id"),
		GroupID:       c.Query("group_id"),
		Limit:         limit,
		Offset:        offset,
	}
	items, total, err := h.service.List(ctx, tenantID, filter)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Totals retorna totais agregados (count, soma de amount_cents) para
// todos os payout_orders que batem com os filtros, independente da
// paginação. Os totais cobrem o conjunto completo de registros, não
// apenas os primeiros N visíveis na listagem (FIN-COM-13).
func (h *PayoutOrderHandler) Totals(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	filter := payout_order.ListFilter{
		Status:        c.Query("status"),
		SourceType:    c.Query("source_type"),
		SourceID:      c.Query("source_id"),
		ContractID:    c.Query("contract_id"),
		RecipientType: c.Query("recipient_type"),
		RecipientID:   c.Query("recipient_id"),
		GroupID:       c.Query("group_id"),
	}
	totals, err := h.service.Totals(ctx, tenantID, filter)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": totals})
}

func (h *PayoutOrderHandler) GetByID(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	order, err := h.service.GetByID(ctx, tenantID, c.Param("id"))
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if order == nil {
		response.NotFound(c, "payout order not found")
		return
	}
	// A tela precisa do valor da comissão que originou a ordem (bruto →
	// descontos → líquido): sem isso o operador só vê o número final.
	summary, summaryErr := h.service.CommissionSummaryForOrder(ctx, order)
	if summaryErr != nil {
		log.Printf("payout order %s: commission summary: %v", order.ID, summaryErr)
	}
	c.JSON(http.StatusOK, gin.H{"data": order, "commission": summary})
}

func (h *PayoutOrderHandler) Approve(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	actor, _ := context.GetAccountID(ctx)
	idempotencyKey := getPayoutOrderIdempotencyKey(c)
	// P1/A13: criador ≠ aprovador, com dispensa para quem responde pela
	// operação financeira (papel autenticado, nunca o corpo).
	ctx = withFinanceAuthorityWaiver(ctx)
	var order *payout_order.PayoutOrder
	if idempotencyKey != "" {
		order, err = h.service.ApproveIdempotent(ctx, tenantID, c.Param("id"), actor)
	} else {
		order, err = h.service.Approve(ctx, tenantID, c.Param("id"), actor)
	}
	if err != nil {
		respondPayoutOrderLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

// MarkPaidRequest captures the off-platform settlement details. The file
// upload itself is handled separately (the front uploads to storage and the URL
// lands here).
//
// ProofURL não tem `binding:"required"` porque a exigência é configurável por
// tenant: quem decide é requireManualProof, consultando a política.
type MarkPaidRequest struct {
	Method        string `json:"method" binding:"required,oneof=MANUAL_PIX MANUAL_TED OTHER"`
	ProofURL      string `json:"proof_url"`
	ProofFilename string `json:"proof_filename"`
	Note          string `json:"note"`
}

func (h *PayoutOrderHandler) MarkPaid(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req MarkPaidRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	// Confirmação manual: registra um pagamento feito fora do sistema, então a
	// evidência é o que separa "paguei" de "pretendo pagar".
	if err := requireManualProof(ctx, h.proofPolicy, tenantID, req.ProofURL); err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	actor, _ := context.GetAccountID(ctx)
	idempotencyKey := getPayoutOrderIdempotencyKey(c)
	// P1/A13: pagador ≠ aprovador.
	ctx = withFinanceAuthorityWaiver(ctx)
	in := service.MarkPaidInput{
		Method:        req.Method,
		ProofURL:      req.ProofURL,
		ProofFilename: req.ProofFilename,
		Note:          req.Note,
	}
	var order *payout_order.PayoutOrder
	if idempotencyKey != "" {
		order, err = h.service.MarkPaidIdempotent(ctx, tenantID, c.Param("id"), actor, in)
	} else {
		order, err = h.service.MarkPaid(ctx, tenantID, c.Param("id"), actor, in)
	}
	if err != nil {
		respondPayoutOrderLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

type CancelRequest struct {
	Reason string `json:"reason"`
}

func (h *PayoutOrderHandler) Cancel(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req CancelRequest
	_ = c.ShouldBindJSON(&req) // reason is optional
	actor, _ := context.GetAccountID(ctx)
	idempotencyKey := getPayoutOrderIdempotencyKey(c)
	var order *payout_order.PayoutOrder
	if idempotencyKey != "" {
		order, err = h.service.CancelIdempotent(ctx, tenantID, c.Param("id"), actor, req.Reason)
	} else {
		order, err = h.service.Cancel(ctx, tenantID, c.Param("id"), actor, req.Reason)
	}
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

// Regenerate recria a ordem cancelada (e reabre a obrigação vinculada), para o
// operador lançar o repasse de novo depois de cancelar. 409 quando a ordem não
// está cancelada — regerar uma ordem viva criaria uma segunda obrigação.
func (h *PayoutOrderHandler) Regenerate(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	actor, _ := context.GetAccountID(ctx)
	order, err := h.service.Regenerate(ctx, tenantID, c.Param("id"), actor)
	if err != nil {
		if errors.Is(err, payout_order.ErrNotCancelled) {
			response.Conflict(c, "só uma ordem cancelada pode ser regerada")
			return
		}
		response.ValidationError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": order})
}

type GroupRequest struct {
	IDs []string `json:"ids" binding:"required,min=1"`
}

// Group is the "agrupar selecionadas em uma ordem consolidada" verb. The
// server generates the group_id and stamps it on every order that's in an
// assignable status (PENDING_REVIEW or APPROVED). Orders in other statuses
// are silently skipped — the client gets back only those that were
// updated.
func (h *PayoutOrderHandler) Group(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req GroupRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	actor, _ := context.GetAccountID(ctx)
	idempotencyKey := getPayoutOrderIdempotencyKey(c)
	var groupID string
	var updated []*payout_order.PayoutOrder
	if idempotencyKey != "" {
		ids := append([]string(nil), req.IDs...)
		sort.Strings(ids)
		groupID = events.DeriveNotificationID(idempotencyKey, "payout_order.group", tenantID, strings.Join(ids, ","))
		groupID, updated, err = h.service.GroupWithID(ctx, tenantID, req.IDs, actor, groupID)
	} else {
		groupID, updated, err = h.service.Group(ctx, tenantID, req.IDs, actor)
	}
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"group_id": groupID,
		"data":     updated,
	})
}

// RecomputeForCharge re-runs orchestration for one charge. Useful when
// the contract's commission percentages were corrected after the charge
// was paid; existing PENDING orders for the charge are cancelled and a
// fresh set is emitted.
func (h *PayoutOrderHandler) RecomputeForCharge(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	if h.orchestration == nil {
		response.InternalServerError(c, "orchestration service not configured")
		return
	}
	actor, _ := context.GetAccountID(ctx)
	cancelled, err := h.orchestration.RecomputeForCharge(ctx, tenantID, c.Param("id"), actor)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": cancelled})
}

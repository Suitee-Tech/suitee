package handlers

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/withdrawal"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// Force usage of the withdrawal package for Swagger.
var _ = withdrawal.Withdrawal{}

type WithdrawalHandler struct {
	service *service.WithdrawalService
}

func NewWithdrawalHandler(svc *service.WithdrawalService) *WithdrawalHandler {
	return &WithdrawalHandler{service: svc}
}

func getWithdrawalIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := strings.TrimSpace(c.GetHeader("Idempotency-Key")); v != "" {
		return v
	}
	return strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
}

type RequestWithdrawalRequest struct {
	AmountCents int64  `json:"amount_cents" binding:"required,gt=0"`
	Description string `json:"description"`
}

// Request godoc
// @Summary      Request an agency withdrawal
// @Description  Requests a withdrawal of the agency's balance ("saque da imobiliária"). Created PENDING; an approver must approve it.
// @Tags         withdrawals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.RequestWithdrawalRequest true "Withdrawal Request"
// @Success      201  {object}  response.Response{data=withdrawal.Withdrawal}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /withdrawals [post]
func (h *WithdrawalHandler) Request(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req RequestWithdrawalRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	actor, _ := context.GetAccountID(c.Request.Context())
	idempotencyKey := getWithdrawalIdempotencyKey(c)
	var w *withdrawal.Withdrawal
	if idempotencyKey != "" {
		w, err = h.service.RequestIdempotent(c.Request.Context(), tenantID, req.AmountCents, req.Description, actor, idempotencyKey)
	} else {
		w, err = h.service.Request(c.Request.Context(), tenantID, req.AmountCents, req.Description, actor)
	}
	if err != nil {
		h.writeErr(c, err)
		return
	}
	response.Created(c, "Withdrawal requested", w)
}

// List godoc
// @Summary      List withdrawals
// @Tags         withdrawals
// @Produce      json
// @Security     BearerAuth
// @Param        status query string false "Status filter"
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response{data=[]withdrawal.Withdrawal}
// @Router       /withdrawals [get]
func (h *WithdrawalHandler) List(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	items, total, err := h.service.List(c.Request.Context(), tenantID, c.Query("status"), limit, offset)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if limit <= 0 {
		limit = 50
	}
	page := int((offset / limit) + 1)
	response.Paginated(c, "Withdrawals retrieved successfully", items, page, int(limit), total)
}

// GetByID godoc
// @Summary      Get a withdrawal by id
// @Tags         withdrawals
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Withdrawal ID"
// @Success      200  {object}  response.Response{data=withdrawal.Withdrawal}
// @Failure      404  {object}  response.Response
// @Router       /withdrawals/{id} [get]
func (h *WithdrawalHandler) GetByID(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	w, err := h.service.GetByID(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		h.writeErr(c, err)
		return
	}
	response.Success(c, "Withdrawal retrieved successfully", w)
}

// Approve godoc
// @Summary      Approve a withdrawal
// @Description  Approves a PENDING withdrawal, moving it to APPROVED so an operator can settle it manually via /submit.
// @Tags         withdrawals
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Withdrawal ID"
// @Success      200  {object}  response.Response{data=withdrawal.Withdrawal}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /withdrawals/{id}/approve [post]
func (h *WithdrawalHandler) Approve(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	actor := h.actor(c)
	idempotencyKey := getWithdrawalIdempotencyKey(c)
	// P1/A13: criador ≠ aprovador, com dispensa para quem responde pela
	// operação financeira (papel autenticado, nunca o corpo).
	ctx := withFinanceAuthorityWaiver(c.Request.Context())
	var w *withdrawal.Withdrawal
	if idempotencyKey != "" {
		w, err = h.service.ApproveIdempotent(ctx, tenantID, c.Param("id"), actor)
	} else {
		w, err = h.service.Approve(ctx, tenantID, c.Param("id"), actor)
	}
	if err != nil {
		h.writeErr(c, err)
		return
	}
	response.Success(c, "Withdrawal approved", w)
}

// SettleWithdrawalRequest carries the operator-supplied evidence of the
// off-platform transfer. Both fields are optional: external_ref is the
// PIX/TED reference (overwrites the auto-generated id) and proof_url is the
// comprovante.
type SettleWithdrawalRequest struct {
	ExternalRef string `json:"external_ref"`
	ProofURL    string `json:"proof_url"`
}

// Submit godoc
// @Summary      Settle an approved withdrawal manually
// @Description  Marks an APPROVED withdrawal as settled off-platform (PIX/TED). Records an optional external reference + proof URL. No payment gateway is involved.
// @Tags         withdrawals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Withdrawal ID"
// @Param        request body handlers.SettleWithdrawalRequest false "Manual settlement evidence"
// @Success      200  {object}  response.Response{data=withdrawal.Withdrawal}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /withdrawals/{id}/submit [post]
func (h *WithdrawalHandler) Submit(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req SettleWithdrawalRequest
	_ = c.ShouldBindJSON(&req) // both fields optional
	idempotencyKey := getWithdrawalIdempotencyKey(c)
	// P1/A13: pagador ≠ aprovador.
	ctx := withFinanceAuthorityWaiver(c.Request.Context())
	var w *withdrawal.Withdrawal
	if idempotencyKey != "" {
		w, err = h.service.SettleIdempotent(ctx, tenantID, c.Param("id"), h.actor(c), req.ExternalRef, req.ProofURL)
	} else {
		w, err = h.service.Settle(ctx, tenantID, c.Param("id"), h.actor(c), req.ExternalRef, req.ProofURL)
	}
	if err != nil {
		h.writeErr(c, err)
		return
	}
	response.Success(c, "Withdrawal settled", w)
}

// Cancel godoc
// @Summary      Cancel a withdrawal
// @Tags         withdrawals
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Withdrawal ID"
// @Success      200  {object}  response.Response{data=withdrawal.Withdrawal}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /withdrawals/{id}/cancel [post]
func (h *WithdrawalHandler) Cancel(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	idempotencyKey := getWithdrawalIdempotencyKey(c)
	var w *withdrawal.Withdrawal
	if idempotencyKey != "" {
		w, err = h.service.CancelIdempotent(c.Request.Context(), tenantID, c.Param("id"), h.actor(c))
	} else {
		w, err = h.service.Cancel(c.Request.Context(), tenantID, c.Param("id"), h.actor(c))
	}
	if err != nil {
		h.writeErr(c, err)
		return
	}
	response.Success(c, "Withdrawal cancelled", w)
}

func (h *WithdrawalHandler) actor(c *gin.Context) string {
	actor, _ := context.GetAccountID(c.Request.Context())
	if actor == "" {
		return "system"
	}
	return actor
}

func (h *WithdrawalHandler) writeErr(c *gin.Context, err error) {
	switch {
	case err.Error() == "withdrawal not found":
		response.NotFound(c, err.Error())
	// Recusa de autoridade (criador ≠ aprovador / aprovador ≠ pagador) é 409,
	// não 500: sem este caso a segregação de funções do saque caía no default e
	// o operador recebia "erro interno" — sem causa visível, porque o
	// sanitizador de erro interno não devolve a mensagem original.
	case isSegregationOfDutiesError(err):
		response.Conflict(c, err.Error())
	case errors.Is(err, withdrawal.ErrAmountTooLow),
		errors.Is(err, withdrawal.ErrNotPending),
		errors.Is(err, withdrawal.ErrNotApproved),
		errors.Is(err, withdrawal.ErrNotCancelable),
		errors.Is(err, withdrawal.ErrInsufficientBalance):
		response.ValidationError(c, err.Error())
	default:
		respondInternalError(c, err)
	}
}

package handlers

import (
	stdcontext "context"
	"errors"
	"strconv"
	"strings"
	"time"

	"net/http"

	"github.com/gin-gonic/gin"
	appcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

const (
	defaultManagedBillLimit int64 = 50
	maxManagedBillLimit     int64 = 100
)

// managedBillService keeps tenant scope explicit and makes the HTTP contract
// independently testable. ManagedBillService satisfies it directly.
type managedBillService interface {
	GetByID(stdcontext.Context, string, string) (*managed_bill.ManagedBill, error)
	List(stdcontext.Context, string, managed_bill.Filter) ([]*managed_bill.ManagedBill, int64, error)
	Approve(stdcontext.Context, string, string, string, int64) (*service.ManagedBillMutationResult, error)
	Pay(stdcontext.Context, string, string, string, int64, string, string, time.Time) (*service.ManagedBillMutationResult, error)
	Reconcile(stdcontext.Context, string, string, string, int64, string, time.Time) (*service.ManagedBillMutationResult, error)
	ReversePayment(stdcontext.Context, string, string, string, int64, string, string, string, time.Time) (*service.ManagedBillMutationResult, error)
	Cancel(stdcontext.Context, string, string, string, int64, string) (*service.ManagedBillMutationResult, error)
}

type ManagedBillHandler struct {
	service managedBillService
}

func NewManagedBillHandler(s managedBillService) *ManagedBillHandler {
	return &ManagedBillHandler{service: s}
}

type ManagedBillVersionedRequest struct {
	ExpectedVersion int64 `json:"expected_version" binding:"required,gt=0"`
}

type PayManagedBillRequest struct {
	ExpectedVersion  int64  `json:"expected_version" binding:"required,gt=0"`
	PaymentReference string `json:"payment_reference" binding:"required"`
	ProofURL         string `json:"proof_url"`
}

type ReconcileManagedBillRequest struct {
	ExpectedVersion         int64  `json:"expected_version" binding:"required,gt=0"`
	ReconciliationReference string `json:"reconciliation_reference" binding:"required"`
}

type ReverseManagedBillPaymentRequest struct {
	ExpectedVersion   int64  `json:"expected_version" binding:"required,gt=0"`
	ReversalReference string `json:"reversal_reference" binding:"required"`
	Reason            string `json:"reason" binding:"required"`
	ProofURL          string `json:"proof_url"`
}

type CancelManagedBillRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required,gt=0"`
	Reason          string `json:"reason" binding:"required"`
}

// List returns concrete owner-paid obligations. UtilityAccount remains the
// recurring configuration and is deliberately not mixed into this result.
func (h *ManagedBillHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := managedBillTenant(c)
	if !ok {
		return
	}
	limit, valid := parseManagedBillQueryInt(c, "limit", defaultManagedBillLimit)
	if !valid {
		return
	}
	offset, valid := parseManagedBillQueryInt(c, "offset", 0)
	if !valid {
		return
	}
	if limit <= 0 || limit > maxManagedBillLimit {
		response.ValidationError(c, "limit must be between 1 and 100")
		return
	}
	if offset < 0 {
		response.ValidationError(c, "offset must be zero or greater")
		return
	}

	status := managed_bill.Status(strings.TrimSpace(c.Query("status")))
	if status != "" && !validManagedBillStatus(status) {
		response.ValidationError(c, "invalid managed bill status")
		return
	}
	filter := managed_bill.Filter{
		SourceTransferID: strings.TrimSpace(c.Query("source_transfer_id")),
		ContractID:       strings.TrimSpace(c.Query("contract_id")),
		PropertyID:       strings.TrimSpace(c.Query("property_id")),
		OwnerID:          strings.TrimSpace(c.Query("owner_id")),
		CompetenceMonth:  strings.TrimSpace(c.Query("competence_month")),
		Status:           status,
		Search:           strings.TrimSpace(c.Query("search")),
		Limit:            limit,
		Offset:           offset,
	}
	items, total, err := h.service.List(ctx, tenantID, filter)
	if err != nil {
		respondManagedBillError(c, err)
		return
	}
	page := int((offset / limit) + 1)
	response.Paginated(c, "Managed bills retrieved successfully", items, page, int(limit), total)
}

func (h *ManagedBillHandler) GetByID(c *gin.Context) {
	tenantID, ok := managedBillTenant(c)
	if !ok {
		return
	}
	bill, err := h.service.GetByID(c.Request.Context(), tenantID, strings.TrimSpace(c.Param("id")))
	if err != nil {
		respondManagedBillError(c, err)
		return
	}
	response.Success(c, "Managed bill retrieved successfully", bill)
}

func (h *ManagedBillHandler) Approve(c *gin.Context) {
	var req ManagedBillVersionedRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	h.runMutation(c, func(ctx stdcontext.Context, tenantID, id, actor string) (*service.ManagedBillMutationResult, error) {
		return h.service.Approve(ctx, tenantID, id, actor, req.ExpectedVersion)
	})
}

func (h *ManagedBillHandler) Pay(c *gin.Context) {
	var req PayManagedBillRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	h.runMutation(c, func(ctx stdcontext.Context, tenantID, id, actor string) (*service.ManagedBillMutationResult, error) {
		return h.service.Pay(ctx, tenantID, id, actor, req.ExpectedVersion, req.PaymentReference, req.ProofURL, time.Time{})
	})
}

func (h *ManagedBillHandler) Reconcile(c *gin.Context) {
	var req ReconcileManagedBillRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	h.runMutation(c, func(ctx stdcontext.Context, tenantID, id, actor string) (*service.ManagedBillMutationResult, error) {
		return h.service.Reconcile(ctx, tenantID, id, actor, req.ExpectedVersion, req.ReconciliationReference, time.Time{})
	})
}

func (h *ManagedBillHandler) ReversePayment(c *gin.Context) {
	var req ReverseManagedBillPaymentRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	h.runMutation(c, func(ctx stdcontext.Context, tenantID, id, actor string) (*service.ManagedBillMutationResult, error) {
		return h.service.ReversePayment(
			ctx,
			tenantID,
			id,
			actor,
			req.ExpectedVersion,
			req.ReversalReference,
			req.Reason,
			req.ProofURL,
			time.Time{},
		)
	})
}

func (h *ManagedBillHandler) Cancel(c *gin.Context) {
	var req CancelManagedBillRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	h.runMutation(c, func(ctx stdcontext.Context, tenantID, id, actor string) (*service.ManagedBillMutationResult, error) {
		return h.service.Cancel(ctx, tenantID, id, actor, req.ExpectedVersion, req.Reason)
	})
}

func (h *ManagedBillHandler) runMutation(c *gin.Context, mutate func(stdcontext.Context, string, string, string) (*service.ManagedBillMutationResult, error)) {
	tenantID, ok := managedBillTenant(c)
	if !ok {
		return
	}
	actor, err := appcontext.GetAccountID(c.Request.Context())
	if err != nil || strings.TrimSpace(actor) == "" {
		response.Forbidden(c, "account context required for managed bill mutation")
		return
	}
	// A dispensa da segregação de funções é decidida aqui, a partir do papel
	// autenticado, e não no serviço: ele não tem como distinguir o papel do ator.
	ctx := withFinanceAuthorityWaiver(c.Request.Context())
	result, err := mutate(ctx, tenantID, strings.TrimSpace(c.Param("id")), strings.TrimSpace(actor))
	if err != nil {
		respondManagedBillError(c, err)
		return
	}
	response.Success(c, "Managed bill updated successfully", result)
}

func managedBillTenant(c *gin.Context) (string, bool) {
	tenantID, err := appcontext.GetTenantID(c.Request.Context())
	if err != nil || strings.TrimSpace(tenantID) == "" {
		response.Forbidden(c, "tenant context required")
		return "", false
	}
	return strings.TrimSpace(tenantID), true
}

func parseManagedBillQueryInt(c *gin.Context, name string, fallback int64) (int64, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		response.ValidationError(c, name+" must be an integer")
		return 0, false
	}
	return value, true
}

func validManagedBillStatus(status managed_bill.Status) bool {
	switch status {
	case managed_bill.StatusReserved, managed_bill.StatusApproved, managed_bill.StatusPaid,
		managed_bill.StatusReconciled, managed_bill.StatusCancelled:
		return true
	default:
		return false
	}
}

func respondManagedBillError(c *gin.Context, err error) {
	// Casos específicos primeiro: os dois são 409, mas o operador precisa saber
	// qual é. Antes caíam no mesmo ErrConcurrentModification genérico.
	var staleVersion *service.ManagedBillStaleVersionError
	var alreadyPaid *service.ManagedBillAlreadyPaidError

	switch {
	case errors.Is(err, managed_bill.ErrNotFound):
		response.NotFound(c, "managed bill not found")
	case errors.As(err, &staleVersion):
		// Corpo com a versão vigente: a interface recarrega e refaz a operação
		// com estado atual, em vez de só exibir o conflito.
		c.JSON(http.StatusConflict, gin.H{
			"success":    false,
			"message":    err.Error(),
			"statusCode": http.StatusConflict,
			"data": gin.H{
				"reason":          "stale_version",
				"current_version": staleVersion.Current,
				"current_status":  string(staleVersion.Status),
			},
		})
	case errors.As(err, &alreadyPaid):
		c.JSON(http.StatusConflict, gin.H{
			"success":    false,
			"message":    err.Error(),
			"statusCode": http.StatusConflict,
			"data": gin.H{
				"reason":            "already_paid",
				"current_status":    string(alreadyPaid.Status),
				"payment_reference": alreadyPaid.CurrentReference,
			},
		})
	case errors.Is(err, managed_bill.ErrConcurrentModification), errors.Is(err, transfer.ErrConcurrentModification):
		response.Conflict(c, err.Error())
	case errors.Is(err, managed_bill.ErrInvalidTransition),
		errors.Is(err, service.ErrManagedBillTransferNotPending),
		errors.Is(err, service.ErrManagedBillReserveOverallocated),
		errors.Is(err, service.ErrManagedBillIdentityMismatch),
		errors.Is(err, service.ErrManagedBillActorConflict),
		errors.Is(err, service.ErrManagedBillReversalActorConflict):
		response.Conflict(c, err.Error())
	case errors.Is(err, managed_bill.ErrInvalidInput):
		response.ValidationError(c, err.Error())
	default:
		respondInternalError(c, err)
	}
}

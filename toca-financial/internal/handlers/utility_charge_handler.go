package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/utility_account"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// UtilityChargeHandler emits a standalone TypeUtility cobrança from a
// VARIABLE_SEPARATE UtilityAccount (água/luz/gás measured by consumption).
// Bundled accounts are billed inside the parent rent charge — they don't
// flow through this endpoint.
type UtilityChargeHandler struct {
	chargeService   *service.ChargeService
	utilityService  *service.UtilityAccountService
	transferService *service.TransferService
	txService       *service.TransactionService
}

func NewUtilityChargeHandler(chargeService *service.ChargeService, utilityService *service.UtilityAccountService, transferService *service.TransferService, txService *service.TransactionService) *UtilityChargeHandler {
	return &UtilityChargeHandler{chargeService: chargeService, utilityService: utilityService, transferService: transferService, txService: txService}
}

type UtilityChargeRequest struct {
	// ReferenceMonth in YYYY-MM format. Used as the charge's reference
	// and as part of the deterministic source_event_id so re-submissions
	// for the same month collapse instead of duplicating the bill.
	ReferenceMonth string    `json:"reference_month" binding:"required"`
	AmountCents    int64     `json:"amount_cents" binding:"required,gt=0"`
	DueDate        time.Time `json:"due_date" binding:"required"`
	Description    string    `json:"description"`
}

func (h *UtilityChargeHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	utilityID := c.Param("id")
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req UtilityChargeRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	ua, err := h.utilityService.GetByID(ctx, tenantID, utilityID)
	if err != nil || ua == nil {
		response.NotFound(c, "utility account not found")
		return
	}
	if ua.BillingMode != utility_account.BillingModeVariableSeparate {
		response.ValidationError(c, "utility account is not in VARIABLE_SEPARATE mode")
		return
	}
	desc := req.Description
	if desc == "" {
		desc = fmt.Sprintf("%s — %s", ua.Provider, req.ReferenceMonth)
	}
	sourceEventID := fmt.Sprintf("utility.%s.%s", ua.ID, req.ReferenceMonth)
	amount := float64(req.AmountCents) / 100.0
	switch ua.ResponsibleParty {
	case utility_account.ResponsibleOwner:
		err := h.transferService.ApplyDeduction(
			ctx, tenantID, ua.ContractID, req.ReferenceMonth, "UTILITY", desc, sourceEventID, amount,
		)
		if err != nil {
			response.ValidationError(c, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"kind": "owner_transfer_deduction", "source_event_id": sourceEventID}})
		return
	case utility_account.ResponsibleAgency:
		tx := transaction.New(ua.ContractID, transaction.TypeDespesa, desc, amount, req.DueDate, tenantID)
		tx.Category = "Conta de consumo"
		// Mantém a despesa no mesmo evento econômico da cobrança/repasse da
		// competência. Sem Month, a caixa de entrada exibia a conta em um cartão
		// isolado e escondia seu efeito no saldo efetivo da imobiliária.
		tx.Month = req.ReferenceMonth
		if err := h.txService.CreateWithSourceEventID(ctx, sourceEventID, tx); err != nil {
			response.ValidationError(c, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"kind": "agency_expense", "source_event_id": sourceEventID}})
		return
	}
	created, err := h.chargeService.CreateWithSourceEventID(
		ctx, sourceEventID, ua.ContractID, "", "",
		"", "", ua.PropertyID, tenantID,
		charge.TypeUtility, desc, req.ReferenceMonth, amount, req.DueDate,
	)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": created})
}

package handlers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type TransactionHandler struct {
	service     *service.TransactionService
	proofPolicy proofPolicySettings
}

// SetProofPolicy conecta a política de comprovante do tenant para a baixa
// manual de lançamentos. Sem isso vale o default do domínio, que exige.
func (h *TransactionHandler) SetProofPolicy(settings proofPolicySettings) {
	h.proofPolicy = settings
}

func NewTransactionHandler(svc *service.TransactionService) *TransactionHandler {
	return &TransactionHandler{service: svc}
}

func respondTransactionLifecycleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, transaction.ErrTransactionNotFound):
		response.NotFound(c, "transaction not found")
	case errors.Is(err, transaction.ErrInvalidStatusTransition), errors.Is(err, transaction.ErrConcurrentModification):
		response.Conflict(c, err.Error())
	default:
		respondInternalError(c, err)
	}
}

func getTransactionIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := strings.TrimSpace(c.GetHeader("Idempotency-Key")); v != "" {
		return v
	}
	return strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
}

type CreateTransactionRequest struct {
	ContractID            string             `json:"contract_id"`
	Type                  transaction.Type   `json:"type" binding:"required"`
	Description           string             `json:"description" binding:"required"`
	Value                 float64            `json:"value"`
	Date                  time.Time          `json:"date" binding:"required"`
	Status                transaction.Status `json:"status"`
	DueDate               *time.Time         `json:"due_date,omitempty"`
	PaymentDate           *time.Time         `json:"payment_date,omitempty"`
	PaymentMethod         string             `json:"payment_method,omitempty"`
	Category              string             `json:"category,omitempty"`
	PayerName             string             `json:"payer_name,omitempty"`
	PayerKind             string             `json:"payer_kind,omitempty"`
	Month                 string             `json:"month,omitempty"`
	GrossValue            *float64           `json:"gross_value,omitempty"`
	AdminFee              *float64           `json:"admin_fee,omitempty"`
	MaintenanceDeductions *float64           `json:"maintenance_deductions,omitempty"`
	UtilityDeductions     *float64           `json:"utility_deductions,omitempty"`
	// ID único para idempotência e rastreabilidade — diferencia
	// transações event-driven (contract.signed → UUID derivado) de
	// transações manuais (Finance.tsx → crypto.randomUUID).
	SourceEventID string `json:"source_event_id"`
	// PaymentPlan é a proveniência informacional do plano de pagamentos
	// programado na esteira (ex.: "Entrada + 11x de R$ 8.333,33").
	PaymentPlan string `json:"payment_plan,omitempty"`
}

type UpdateTransactionRequest struct {
	Type                  transaction.Type   `json:"type"`
	Description           string             `json:"description"`
	Value                 *float64           `json:"value"`
	Date                  *time.Time         `json:"date,omitempty"`
	Status                transaction.Status `json:"status"`
	DueDate               *time.Time         `json:"due_date,omitempty"`
	PaymentDate           *time.Time         `json:"payment_date,omitempty"`
	PaymentMethod         string             `json:"payment_method,omitempty"`
	Category              string             `json:"category,omitempty"`
	PayerName             string             `json:"payer_name,omitempty"`
	PayerKind             string             `json:"payer_kind,omitempty"`
	Month                 string             `json:"month,omitempty"`
	GrossValue            *float64           `json:"gross_value,omitempty"`
	AdminFee              *float64           `json:"admin_fee,omitempty"`
	MaintenanceDeductions *float64           `json:"maintenance_deductions,omitempty"`
	UtilityDeductions     *float64           `json:"utility_deductions,omitempty"`
	// PaymentPlan é a proveniência informacional do plano de pagamentos.
	PaymentPlan string `json:"payment_plan,omitempty"`
}

func (h *TransactionHandler) Create(c *gin.Context) {
	var req CreateTransactionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	t := transaction.New(req.ContractID, req.Type, req.Description, req.Value, req.Date, tenantID)
	if req.Status != "" {
		t.Status = transaction.CanonicalStatus(req.Status)
	}
	// Valida campos obrigatórios e enums contra o domínio.
	if err := t.Validate(); err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	t.DueDate = req.DueDate
	t.PaymentDate = req.PaymentDate
	t.PaymentMethod = req.PaymentMethod
	t.Category = req.Category
	t.PayerName = req.PayerName
	t.PayerKind = req.PayerKind
	t.Month = req.Month
	t.GrossValue = req.GrossValue
	t.AdminFee = req.AdminFee
	t.MaintenanceDeductions = req.MaintenanceDeductions
	t.UtilityDeductions = req.UtilityDeductions
	t.PaymentPlan = strings.TrimSpace(req.PaymentPlan)

	// FIN-COM-12: validação cruzada — se gross_value está presente,
	// a soma de admin_fee + maintenance_deductions + utility_deductions
	// não pode exceder o valor bruto.
	if t.GrossValue != nil && *t.GrossValue > 0 {
		var deductions float64
		if t.AdminFee != nil {
			deductions += *t.AdminFee
		}
		if t.MaintenanceDeductions != nil {
			deductions += *t.MaintenanceDeductions
		}
		if t.UtilityDeductions != nil {
			deductions += *t.UtilityDeductions
		}
		if deductions > *t.GrossValue {
			response.ValidationError(c, fmt.Sprintf(
				"deductions (%.2f) exceed gross value (%.2f): admin_fee=%.2f + maintenance=%.2f + utility=%.2f",
				deductions, *t.GrossValue,
				valOrZero(t.AdminFee), valOrZero(t.MaintenanceDeductions), valOrZero(t.UtilityDeductions),
			))
			return
		}
	}

	idempotencyKey := getTransactionIdempotencyKey(c)
	// source_event_id no corpo da requisição tem prioridade sobre o header
	effectiveSourceID := req.SourceEventID
	if effectiveSourceID == "" && idempotencyKey != "" {
		effectiveSourceID = events.DeriveNotificationID(
			idempotencyKey,
			"transaction.create",
			tenantID,
			req.ContractID,
			string(req.Type),
			req.Date.UTC().Format(time.RFC3339),
			strconv.FormatFloat(req.Value, 'f', 2, 64),
			req.Description,
		)
	}
	if effectiveSourceID != "" {
		created, err := h.service.CreateFromRequest(c.Request.Context(), effectiveSourceID, t)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		t = created
	} else {
		if err := h.service.Create(c.Request.Context(), t); err != nil {
			respondInternalError(c, err)
			return
		}
	}

	response.Created(c, "Transaction created successfully", t)
}

func (h *TransactionHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	t, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		respondTransactionLifecycleError(c, err)
		return
	}
	response.Success(c, "Transaction retrieved successfully", t)
}

func (h *TransactionHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateTransactionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		respondTransactionLifecycleError(c, err)
		return
	}

	if existing.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}
	if req.Status != "" {
		if err := transaction.ValidateStatusTransition(existing.Status, req.Status); err != nil {
			response.Conflict(c, err.Error())
			return
		}
	}

	if req.Type != "" {
		existing.Type = req.Type
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Value != nil {
		if *req.Value != 0 {
			existing.Value = *req.Value
		}
	}
	if req.Date != nil {
		existing.Date = *req.Date
	}
	if req.Status != "" {
		existing.Status = transaction.CanonicalStatus(req.Status)
	}
	existing.DueDate = req.DueDate
	existing.PaymentDate = req.PaymentDate
	if req.PaymentMethod != "" {
		existing.PaymentMethod = req.PaymentMethod
	}
	if req.Category != "" {
		existing.Category = req.Category
	}
	if req.PayerName != "" {
		existing.PayerName = req.PayerName
	}
	if req.PayerKind != "" {
		existing.PayerKind = req.PayerKind
	}
	if req.Month != "" {
		existing.Month = req.Month
	}
	existing.GrossValue = req.GrossValue
	existing.AdminFee = req.AdminFee
	existing.MaintenanceDeductions = req.MaintenanceDeductions
	existing.UtilityDeductions = req.UtilityDeductions
	if req.PaymentPlan != "" {
		existing.PaymentPlan = req.PaymentPlan
	}
	if err := existing.Validate(); err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	if err := h.service.Update(c.Request.Context(), existing); err != nil {
		respondTransactionLifecycleError(c, err)
		return
	}

	response.Success(c, "Transaction updated successfully", existing)
}

type SettleTransactionRequest struct {
	PaymentDate   *time.Time `json:"payment_date,omitempty"`
	PaymentMethod string     `json:"payment_method,omitempty"`
	// ProofURL e Note padronizam o comprovante da baixa com o restante do
	// fluxo financeiro: qualquer lançamento pode anexar documento e
	// observação ao ser marcado como pago.
	ProofURL string `json:"proof_url,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Settle records a manual baixa (recebimento/pagamento) on a transaction:
// transitions it to Pago with the given payment date/method.
func (h *TransactionHandler) Settle(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req SettleTransactionRequest
	if !validation.BindJSONOptional(c, &req) {
		return
	}

	if err := requireManualProof(c.Request.Context(), h.proofPolicy, tenantID, req.ProofURL); err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	t, err := h.service.SettleWithProof(c.Request.Context(), id, tenantID, req.PaymentDate, req.PaymentMethod, req.ProofURL, req.Note)
	if err != nil {
		respondTransactionLifecycleError(c, err)
		return
	}

	response.Success(c, "Transaction settled successfully", t)
}

func (h *TransactionHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id, tenantID); err != nil {
		respondTransactionLifecycleError(c, err)
		return
	}

	response.Success(c, "Transaction deleted successfully", nil)
}

type ReverseTransactionRequest struct {
	Reason string `json:"reason" binding:"required,min=3"`
}

func (h *TransactionHandler) Reverse(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req ReverseTransactionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	operatorID, _ := context.GetAccountID(c.Request.Context())

	reversed, err := h.service.Reverse(c.Request.Context(), id, tenantID, req.Reason, operatorID)
	if err != nil {
		respondTransactionLifecycleError(c, err)
		return
	}

	response.Success(c, "Transaction reversed successfully", reversed)
}

// Cancel cancela um lançamento manual ainda não pago (PENDING/OVERDUE), com
// cascade para as comissões vinculadas. Depois do cancelamento o operador pode
// fazer um novo lançamento.
func (h *TransactionHandler) Cancel(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req ReverseTransactionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	cancelled, err := h.service.Cancel(c.Request.Context(), id, tenantID, req.Reason)
	if err != nil {
		respondTransactionLifecycleError(c, err)
		return
	}

	response.Success(c, "Transaction cancelled successfully", cancelled)
}

func (h *TransactionHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	limit, offset, page := normalizePaging(limit, offset)

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	parseTime := func(v string) (*time.Time, error) {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, nil
		}
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return &t, nil
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return &t, nil
		}
		if t, err := timeutil.ParseDateOnly(v); err == nil {
			// Dia civil no fuso configurado: `time.Parse` devolveria meia-noite
			// UTC e o filtro "a partir de 30/09" começaria às 21h do dia 29.
			return &t, nil
		}
		return nil, err
	}

	parseFloatPtr := func(v string) (*float64, error) {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, err
		}
		return &f, nil
	}

	minAmount, err := parseFloatPtr(c.Query("min_amount"))
	if err != nil {
		response.ValidationError(c, "invalid min_amount value")
		return
	}
	maxAmount, err := parseFloatPtr(c.Query("max_amount"))
	if err != nil {
		response.ValidationError(c, "invalid max_amount value")
		return
	}
	dateFrom, err := parseTime(c.Query("date_from"))
	if err != nil {
		response.ValidationError(c, "invalid date_from value")
		return
	}
	dateTo, err := parseTime(c.Query("date_to"))
	if err != nil {
		response.ValidationError(c, "invalid date_to value")
		return
	}

	txType := c.Query("type")
	switch strings.ToLower(strings.TrimSpace(txType)) {
	case "income":
		txType = string(transaction.TypeReceita)
	case "expense":
		txType = string(transaction.TypeDespesa)
	}

	filter := transaction.SearchFilter{
		ContractID:  c.Query("contract_id"),
		Category:    c.Query("category"),
		Type:        txType,
		MinAmount:   minAmount,
		MaxAmount:   maxAmount,
		DateFrom:    dateFrom,
		DateTo:      dateTo,
		Status:      c.Query("status"),
		Description: c.Query("description"),
		Limit:       limit,
		Offset:      offset,
	}

	transactions, total, err := h.service.Search(c.Request.Context(), tenantID, filter)
	if err != nil {
		if database.IsTimeout(err) {
			response.GatewayTimeout(c, "request timeout while listing transactions")
			return
		}
		respondInternalError(c, err)
		return
	}

	response.Paginated(c, "Transactions retrieved successfully", transactions, page, int(limit), total)
}

// valOrZero retorna o valor do ponteiro float64 ou 0 se for nil.
func valOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// BatchTransactionRequest wraps a list of transaction line-items for atomic insertion.
type BatchTransactionRequest struct {
	Items               []service.BatchCreateInput              `json:"items" binding:"required,min=1"`
	Commissions         []service.ManualCommissionInput         `json:"commissions,omitempty"`
	CommissionRetention *service.ManualCommissionRetentionInput `json:"commission_retention,omitempty"`
}

// CreateBatch godoc
// @Summary      Create transactions in batch
// @Description  Insere transações e commissions relacionadas em uma única transação, com IDs idempotentes por linha. Em recebimentos com distribuição parcial, commission_retention explicita percentual e valor retidos pela imobiliária sem criar uma obrigação pagável.
// @Tags         transactions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.BatchTransactionRequest true "Batch"
// @Success      200 {object} response.Response{data=[]service.BatchCreateOutput}
// @Router       /transactions/batch [post]
func (h *TransactionHandler) CreateBatch(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req BatchTransactionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	idempotencyKey := getTransactionIdempotencyKey(c)
	if idempotencyKey == "" {
		response.ValidationError(c, "Idempotency-Key header is required for batch creation")
		return
	}

	outputs, err := h.service.BatchCreatePosting(ctx, tenantID, idempotencyKey, req.Items, req.Commissions, req.CommissionRetention)
	if err != nil {
		if errors.Is(err, transaction.ErrBatchOperationKeyRequired) || errors.Is(err, transaction.ErrBatchValidation) {
			response.ValidationError(c, err.Error())
			return
		}
		if errors.Is(err, transaction.ErrBatchIdempotencyConflict) {
			response.Conflict(c, err.Error())
			return
		}
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Batch created successfully", outputs)
}

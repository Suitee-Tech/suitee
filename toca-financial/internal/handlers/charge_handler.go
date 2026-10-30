package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	stdctx "context"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"

	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// Force usage of charge package for Swagger
var _ = charge.Charge{}

type ChargeHandler struct {
	service *service.ChargeService
	// permissionProvider avalia a permissão dedicada de aprovação de ajuste de
	// valor (M11). Nil ⇒ nega (fail-closed).
	permissionProvider auth.PermissionProvider
	// crmClient resolve o NOME do devedor na listagem (a cobrança guarda só o id
	// da parte). Opcional: sem ele a listagem segue igual, sem o campo.
	crmClient crmLeadLookup
	// tenantCompany resolve o nome da imobiliária impresso nos recibos. Opcional:
	// sem ele o PDF mantém o fallback neutro, nunca o nome de outra empresa.
	tenantCompany tenantCompanyLookup
}

// tenantCompanyLookup é a fatia de leitura da empresa do tenant usada nos
// documentos financeiros (recibo de cobrança e demonstrativo de repasse).
type tenantCompanyLookup interface {
	CompanyName(ctx stdctx.Context, tenantID string) (string, error)
}

// crmLeadLookup é a fatia do cliente CRM usada aqui (uma busca por lead).
type crmLeadLookup interface {
	GetLead(ctx stdctx.Context, tenantID, leadID string) (*clients.LeadDTO, error)
}

// WithCRMClient injeta o cliente usado para resolver o nome do devedor.
func (h *ChargeHandler) WithCRMClient(c crmLeadLookup) *ChargeHandler {
	h.crmClient = c
	return h
}

// WithTenantCompanyLookup injeta o resolvedor do nome da imobiliária logada
// usado no cabeçalho/rodapé do recibo.
func (h *ChargeHandler) WithTenantCompanyLookup(lookup tenantCompanyLookup) *ChargeHandler {
	h.tenantCompany = lookup
	return h
}

func NewChargeHandler(service *service.ChargeService) *ChargeHandler {
	return &ChargeHandler{service: service}
}

// WithPermissionProvider injeta o resolvedor de permissões usado pelo gate de
// aprovação de ajuste (M11).
func (h *ChargeHandler) WithPermissionProvider(provider auth.PermissionProvider) *ChargeHandler {
	h.permissionProvider = provider
	return h
}

func getChargeIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := c.GetHeader("Idempotency-Key"); v != "" {
		return v
	}
	return c.GetHeader("X-Idempotency-Key")
}

// SecurityDepositBalance handles GET /contracts/:id/security-deposit-balance.
// The Held query parameter carries the caução amount from the Contract
// (front office passes it through after fetching the contract). Keeping
// the value in the request avoids a circular service call into toca-deal.
func (h *ChargeHandler) SecurityDepositBalance(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	contractID := c.Param("id")
	if strings.TrimSpace(contractID) == "" {
		response.ValidationError(c, "contract id is required")
		return
	}
	heldRaw := c.DefaultQuery("held", "0")
	held, err := strconv.ParseFloat(heldRaw, 64)
	if err != nil {
		held = 0
	}
	balance, err := h.service.ComputeSecurityDepositBalance(ctx, tenantID, contractID, held)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Security deposit balance retrieved successfully", balance)
}

type CreateChargeRequest struct {
	ContractID     string            `json:"contract_id" binding:"required"`
	TenantPartyID  string            `json:"tenant_party_id" binding:"required"`
	OwnerID        string            `json:"owner_id" binding:"required"`
	PropertyID     string            `json:"property_id" binding:"required"`
	Type           charge.ChargeType `json:"type" binding:"required"`
	Description    string            `json:"description" binding:"required"`
	ReferenceMonth string            `json:"reference_month" binding:"required"`
	Amount         float64           `json:"amount" binding:"required,gt=0"`
	DueDate        time.Time         `json:"due_date" binding:"required"`
	// Discount is an optional reduction applied to the total
	// (TotalAmount = Amount + LateFee + Interest - Discount).
	Discount float64 `json:"discount,omitempty"`
	// LineItems is the optional composition breakdown ("valores inclusos
	// na cobrança": aluguel, condomínio, IPTU, utilidades). When present,
	// Amount is derived from their sum (Σ items == Amount, enforced by
	// charge.ValidateLineItems).
	LineItems []charge.LineItem `json:"line_items,omitempty"`
	DealID    string            `json:"deal_id,omitempty"`
	LeadID    string            `json:"lead_id,omitempty"`
}

type UpdateChargeRequest struct {
	Description    *string            `json:"description,omitempty"`
	Type           *charge.ChargeType `json:"type,omitempty"`
	ReferenceMonth *string            `json:"reference_month,omitempty"`
	Amount         *float64           `json:"amount,omitempty"`
	LateFee        *float64           `json:"late_fee,omitempty"`
	Interest       *float64           `json:"interest,omitempty"`
	Discount       *float64           `json:"discount,omitempty"`
	DueDate        *time.Time         `json:"due_date,omitempty"`
	LineItems      *[]charge.LineItem `json:"line_items,omitempty"`
	Status         *charge.Status     `json:"status,omitempty"`
}

type PayChargeRequest struct {
	PaymentMethod string  `json:"payment_method" binding:"required"`
	PaidAmount    float64 `json:"paid_amount" binding:"required,gt=0"`
}

// Create godoc
// @Summary      Create charge
// @Description  Create a new charge
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateChargeRequest true "Create Charge Request"
// @Success      201  {object}  response.Response{data=charge.Charge}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /charges [post]
func (h *ChargeHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.Create")

	var req CreateChargeRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	idempotencyKey := getChargeIdempotencyKey(c)
	var ch *charge.Charge
	if idempotencyKey != "" {
		sourceEventID := events.DeriveNotificationID(
			idempotencyKey,
			"charge.create",
			req.ContractID,
			string(req.Type),
			req.ReferenceMonth,
			strconv.FormatFloat(req.Amount, 'f', 2, 64),
			req.DueDate.UTC().Format(time.RFC3339),
		)
		ch, err = h.service.CreateWithCompositionFromRequest(ctx, sourceEventID, req.ContractID, req.DealID, req.LeadID, req.TenantPartyID, req.OwnerID, req.PropertyID, tenantID, req.Type, req.Description, req.ReferenceMonth, req.Amount, req.Discount, req.DueDate, req.LineItems)
	} else {
		ch, err = h.service.CreateWithComposition(ctx, req.ContractID, req.DealID, req.LeadID, req.TenantPartyID, req.OwnerID, req.PropertyID, tenantID, req.Type, req.Description, req.ReferenceMonth, req.Amount, req.Discount, req.DueDate, req.LineItems)
	}
	if err != nil {
		log.Error("failed to create charge", "error", err)
		response.ValidationError(c, err.Error())
		return
	}

	log.Info("charge created successfully", "charge_id", ch.ID)
	response.Created(c, "Charge created successfully", ch)
}

// List godoc
// @Summary      List charges
// @Description  List charges with pagination and filters
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        contract_id query string false "Contract ID"
// @Param        status query string false "Status"
// @Param        reference_month query string false "Reference Month"
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response{data=[]charge.Charge}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /charges [get]
func (h *ChargeHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.List")

	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	limit, offset, page := normalizePaging(limit, offset)
	var status string
	if s := c.Query("status"); s != "" {
		status = s
	}

	contractID := c.Query("contract_id")
	referenceMonth := c.Query("reference_month")
	partyIDs := make([]string, 0)
	if v := c.Query("party_id"); v != "" {
		partyIDs = append(partyIDs, strings.TrimSpace(v))
	}
	if v := c.Query("party_ids"); v != "" {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				partyIDs = append(partyIDs, part)
			}
		}
	}
	if len(partyIDs) > 0 {
		seen := map[string]struct{}{}
		uniq := make([]string, 0, len(partyIDs))
		for _, id := range partyIDs {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			uniq = append(uniq, id)
		}
		partyIDs = uniq
	}

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
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
	dueFrom, err := parseTime(c.Query("due_from"))
	if err != nil {
		response.ValidationError(c, "invalid due_from value")
		return
	}
	dueTo, err := parseTime(c.Query("due_to"))
	if err != nil {
		response.ValidationError(c, "invalid due_to value")
		return
	}
	paidFrom, err := parseTime(c.Query("paid_from"))
	if err != nil {
		response.ValidationError(c, "invalid paid_from value")
		return
	}
	paidTo, err := parseTime(c.Query("paid_to"))
	if err != nil {
		response.ValidationError(c, "invalid paid_to value")
		return
	}

	filter := charge.SearchFilter{
		ContractID:     contractID,
		ReferenceMonth: referenceMonth,
		Status:         status,
		PartyIDs:       partyIDs,
		CustomerID:     c.Query("customer_id"),
		CustomerName:   c.Query("customer_name"),
		DealID:         c.Query("deal_id"),
		MinAmount:      minAmount,
		MaxAmount:      maxAmount,
		DueFrom:        dueFrom,
		DueTo:          dueTo,
		PaidFrom:       paidFrom,
		PaidTo:         paidTo,
		PaymentMethod:  c.Query("payment_method"),
		Limit:          limit,
		Offset:         offset,
	}

	charges, total, err := h.service.Search(ctx, tenantID, filter)
	if err != nil {
		if database.IsTimeout(err) {
			log.Warn("charge list timed out", "error", err)
			response.GatewayTimeout(c, "request timeout while listing charges")
			return
		}
		log.Error("failed to list charges", "error", err)
		respondInternalError(c, err)
		return
	}

	// A listagem principal também leva `days_overdue` (mesma regra do backend):
	// é ela que alimenta a coluna de situação do financeiro.
	response.Paginated(c, "Charges retrieved successfully", internalChargesComAtraso(charges, h.debtorNames(c.Request.Context(), tenantID, charges)), page, int(limit), total)
}

// GetByID godoc
// @Summary      Get charge by ID
// @Description  Get a charge by its ID
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Success      200  {object}  response.Response{data=charge.Charge}
// @Failure      404  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /charges/{id} [get]
func (h *ChargeHandler) GetByID(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.GetByID", "charge_id", id)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	ch, err := h.service.GetByID(ctx, tenantID, id)
	if err != nil {
		log.Warn("failed to get charge", "error", err)
		switch {
		case errors.Is(err, charge.ErrNotFound):
			response.NotFound(c, err.Error())
		case errors.Is(err, charge.ErrUnauthorized):
			response.Forbidden(c, err.Error())
		default:
			respondInternalError(c, err)
		}
		return
	}

	response.Success(c, "Charge retrieved successfully", ch)
}

// Update godoc
// @Summary      Update charge
// @Description  Update charge details
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Param        request body handlers.UpdateChargeRequest true "Update Charge Request"
// @Success      200  {object}  response.Response{data=charge.Charge}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /charges/{id} [put]
func (h *ChargeHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.Update", "charge_id", id)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateChargeRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	loaded, err := h.service.GetByID(ctx, tenantID, id)
	if err != nil {
		log.Warn("failed to get charge", "error", err)
		if errors.Is(err, charge.ErrNotFound) {
			response.NotFound(c, err.Error())
		} else {
			respondInternalError(c, err)
		}
		return
	}
	// M11: o estado ANTERIOR é a referência do tamanho do ajuste.
	before := *loaded
	existing := loaded

	if req.Status != nil && *req.Status != existing.Status {
		response.ValidationError(c, "status must be changed through a dedicated lifecycle endpoint")
		return
	}
	financialEdit := req.Type != nil || req.ReferenceMonth != nil || req.Amount != nil || req.LateFee != nil || req.Interest != nil || req.Discount != nil || req.DueDate != nil || req.LineItems != nil
	if financialEdit && existing.Status != charge.StatusPending && existing.Status != charge.StatusOverdue {
		response.ValidationError(c, "financial fields can only be edited while charge is PENDING or OVERDUE")
		return
	}
	for field, value := range map[string]*float64{
		"late_fee": req.LateFee,
		"interest": req.Interest,
		"discount": req.Discount,
	} {
		if value != nil && *value < 0 {
			response.ValidationError(c, field+" cannot be negative")
			return
		}
	}
	if req.Amount != nil && *req.Amount <= 0 {
		response.ValidationError(c, "amount must be greater than zero")
		return
	}

	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.Type != nil {
		existing.Type = *req.Type
	}
	if req.ReferenceMonth != nil {
		if strings.TrimSpace(*req.ReferenceMonth) == "" {
			response.ValidationError(c, "reference_month cannot be empty")
			return
		}
		existing.ReferenceMonth = strings.TrimSpace(*req.ReferenceMonth)
	}
	if req.Amount != nil {
		existing.Amount = valueobjects.NewMoneyFromFloat(*req.Amount)
	}
	if req.LateFee != nil {
		existing.LateFee = valueobjects.NewMoneyFromFloat(*req.LateFee)
	}
	if req.Interest != nil {
		existing.Interest = valueobjects.NewMoneyFromFloat(*req.Interest)
	}
	if req.Discount != nil {
		existing.Discount = valueobjects.NewMoneyFromFloat(*req.Discount)
	}
	if req.DueDate != nil {
		if req.DueDate.IsZero() {
			response.ValidationError(c, "due_date cannot be empty")
			return
		}
		existing.DueDate = *req.DueDate
	}
	if req.LineItems != nil {
		if err := existing.SetLineItems(*req.LineItems); err != nil {
			response.ValidationError(c, err.Error())
			return
		}
	} else if err := charge.ValidateLineItems(existing.LineItems, existing.Amount.Cents()); err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	existing.CalculateTotal()
	if existing.TotalAmount.Cents() <= 0 {
		response.ValidationError(c, "charge total must be greater than zero")
		return
	}

	// M11 — waive/adjust acima do limiar exige segunda assinatura
	// (`finance:approve_adjustment`). A permissão de escrita do dia a dia
	// (`charges:write`) concede o ajuste pequeno; o grande precisa de aprovador.
	magnitude := chargeAdjustmentMagnitude(before, *existing)
	threshold := adjustmentApprovalThresholdBRL()
	requiresApproval := adjustmentRequiresApproval(magnitude, threshold)
	approved := false
	if requiresApproval {
		if !actorCanApproveAdjustment(c, h.permissionProvider) {
			audit.Record(ctx, audit.Entry{
				Action:     "finance.charge_adjustment_denied",
				EntityType: "charge",
				EntityID:   id,
				Before:     map[string]any{"total": before.TotalAmount.Float()},
				After:      map[string]any{"total": existing.TotalAmount.Float()},
				Metadata: map[string]any{
					"tenant_id":           tenantID,
					"magnitude_brl":       magnitude,
					"threshold_brl":       threshold,
					"required_permission": AdjustmentApprovalPermission,
				},
			})
			log.Warn("charge adjustment denied by approval gate",
				"magnitude_brl", magnitude, "threshold_brl", threshold)
			response.Error(c, http.StatusForbidden, fmt.Sprintf(
				"ajuste de %s excede o limiar de %s e exige a permissão %s",
				money.BRL(magnitude), money.BRL(threshold), AdjustmentApprovalPermission))
			return
		}
		approved = true
	}

	if err := h.service.Update(ctx, existing); err != nil {
		log.Error("failed to update charge", "error", err)
		if errors.Is(err, charge.ErrConcurrentModification) {
			response.Conflict(c, err.Error())
			return
		}
		respondInternalError(c, err)
		return
	}

	// Trilha do ajuste: quem mexeu, quanto e com que autoridade. Sem isso não há
	// como reconstituir uma concessão de valor (M11 + M5).
	if financialEdit {
		audit.Record(ctx, audit.Entry{
			Action:     "finance.charge_adjusted",
			EntityType: "charge",
			EntityID:   id,
			Before:     map[string]any{"total": before.TotalAmount.Float(), "amount": before.Amount.Float(), "discount": before.Discount.Float(), "late_fee": before.LateFee.Float(), "interest": before.Interest.Float()},
			After:      map[string]any{"total": existing.TotalAmount.Float(), "amount": existing.Amount.Float(), "discount": existing.Discount.Float(), "late_fee": existing.LateFee.Float(), "interest": existing.Interest.Float()},
			Metadata: map[string]any{
				"tenant_id":         tenantID,
				"magnitude_brl":     magnitude,
				"threshold_brl":     threshold,
				"approved":          approved,
				"approval_required": requiresApproval,
			},
		})
	}

	log.Info("charge updated successfully")
	response.Success(c, "Charge updated successfully", existing)
}

// Pay godoc
// @Summary      Pay charge
// @Description  Mark a charge as paid
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Param        request body handlers.PayChargeRequest true "Pay Charge Request"
// @Success      200  {object}  response.Response{data=charge.Charge}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /charges/{id}/pay [post]
func (h *ChargeHandler) Pay(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.Pay", "charge_id", id)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	var req PayChargeRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	idempotencyKey := strings.TrimSpace(getChargeIdempotencyKey(c))
	if idempotencyKey == "" {
		response.ValidationError(c, "Idempotency-Key header is required")
		return
	}
	paymentID := events.DeriveNotificationID("charge.payment", tenantID, id, idempotencyKey)

	// Unificado com ReconcileManual: ambos passam por MarkPaid
	// que acumula pagamentos (FIN-E2E-18), dispara transfer hook,
	// publica charge.paid na quitação, sincroniza transactions e grava audit.
	updated, err := h.service.MarkPaidIdempotent(ctx, tenantID, id, req.PaymentMethod, req.PaidAmount, paymentID, time.Now())
	if err != nil {
		log.Warn("failed to pay charge", "error", err)
		switch {
		case errors.Is(err, charge.ErrNotFound):
			response.NotFound(c, err.Error())
		case errors.Is(err, charge.ErrInvalidState), errors.Is(err, charge.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		default:
			respondInternalError(c, err)
		}
		return
	}

	log.Info("charge paid successfully")
	response.Success(c, "Charge paid successfully", updated)
}

// Notify godoc
// @Summary      Send reminder for a charge
// @Description  Dispatches an email + in-app reminder for the charge to the payer lead.
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Success      200  {object}  response.Response{data=service.NotifyResult}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /charges/{id}/notify [post]
func (h *ChargeHandler) Notify(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.Notify", "id", c.Param("id"))

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	actor, _ := context.GetAccountID(ctx)
	sentBy := ""
	if actor != "" {
		sentBy = "user:" + actor
	}
	sentByLabel, _ := context.GetUserName(ctx)
	res, err := h.service.NotifyWithOptions(ctx, tenantID, c.Param("id"), service.NotifyOptions{
		SentBy:      sentBy,
		SentByLabel: sentByLabel,
	})
	if err != nil {
		log.Info("notify failed", "error", err)
		switch {
		case errors.Is(err, charge.ErrNotFound):
			response.NotFound(c, err.Error())
		case errors.Is(err, service.ErrCommClientNotConfigured), errors.Is(err, service.ErrCrmClientNotConfigured):
			respondInternalError(c, err)
		default:
			response.ValidationError(c, err.Error())
		}
		return
	}
	response.Success(c, "Reminder dispatched", res)
}

// ListOverdue godoc
// @Summary      List overdue charges
// @Description  List all overdue charges for a tenant
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]charge.Charge}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /charges/overdue [get]
func (h *ChargeHandler) ListOverdue(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.ListOverdue")

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	charges, err := h.service.ListOverdue(ctx, tenantID)
	if err != nil {
		log.Error("failed to list overdue charges", "error", err)
		respondInternalError(c, err)
		return
	}

	// `days_overdue` acompanha a lista: a regra de atraso (vence no dia, atrasa no
	// dia seguinte, no fuso do tenant) fica SÓ no backend. Sem isso cada tela
	// reimplementava a conta e a do próprio dia aparecia como vencida.
	response.Success(c, "Overdue charges retrieved successfully", internalChargesComAtraso(charges, h.debtorNames(c.Request.Context(), tenantID, charges)))
}

func (h *ChargeHandler) ListInternalCharges(c *gin.Context) {
	dealID := c.Param("id")
	accountID := c.Query("account_id")
	role := c.Query("role")

	if dealID == "" {
		response.ValidationError(c, "deal ID is required")
		return
	}

	// SECURITY (achado A3): o tenant vem da CREDENCIAL (JWT do usuário ou chave
	// de serviço com allowlist), nunca do chamador.
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

	charges, err := h.service.ListInternalCharges(c.Request.Context(), tenantID, dealID, accountID, role)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	// `days_overdue` é MÉTODO no domínio (`Charge.DaysOverdue()`), e método não
	// serializa: quem consome esta rota (portal do cliente, via toca-deal) só
	// tinha `due_date` e precisava recalcular o atraso por conta própria — duas
	// réguas para o mesmo número. Vai como campo de RESPOSTA (DTO), não no struct
	// do domínio: o repositório persiste o struct inteiro no metadata JSONB e um
	// campo calculado ficaria congelado no banco.
	response.Success(c, "Charges retrieved successfully", internalChargesComAtraso(charges, h.debtorNames(c.Request.Context(), tenantID, charges)))
}

// internalChargeResponse é a visão de cobrança para chamadores internos
// (portal do cliente): os campos do domínio + o atraso calculado.
type internalChargeResponse struct {
	*charge.Charge
	DaysOverdue int `json:"days_overdue"`
	// DebtorName é o nome de quem deve, resolvido no CRM por `tenant_party_id`.
	// A cobrança guarda o ID da parte (não o nome), então a tela mostrava um UUID
	// onde o operador espera um nome. Vem vazio quando a resolução não é possível
	// (sem CRM configurado, lead removido, limite de consultas) — a tela cai no ID.
	DebtorName string `json:"debtor_name,omitempty"`
}

// debtorNames resolve o nome de quem deve para as cobranças da lista.
//
// A cobrança guarda `tenant_party_id` (o lead), não o nome — por isso a tela
// mostrava um UUID na coluna do inquilino. O nome existe no CRM e é buscado aqui,
// no servidor, porque:
//
//   - o front não tem (nem deve ter) acesso ao CRM;
//   - resolver no cliente seria uma chamada por linha, sem cache;
//   - o mesmo nome serve para o operador conferir e para montar o payload de
//     abertura manual de caso (que exige `debtor_name`).
//
// Limites deliberados: no máximo `maxDebtorLookups` devedores distintos por
// requisição, com concorrência de 4 e falha silenciosa (nome ausente na resposta,
// nunca erro na listagem — o CRM fora do ar não pode derrubar o financeiro).
func (h *ChargeHandler) debtorNames(ctx stdctx.Context, tenantID string, charges []*charge.Charge) map[string]string {
	if h.crmClient == nil || tenantID == "" {
		return nil
	}
	const maxDebtorLookups = 20

	ids := make([]string, 0, maxDebtorLookups)
	vistos := make(map[string]struct{}, maxDebtorLookups)
	for _, c := range charges {
		if c == nil || strings.TrimSpace(c.TenantPartyID) == "" {
			continue
		}
		if _, ok := vistos[c.TenantPartyID]; ok {
			continue
		}
		if len(ids) >= maxDebtorLookups {
			break
		}
		vistos[c.TenantPartyID] = struct{}{}
		ids = append(ids, c.TenantPartyID)
	}
	if len(ids) == 0 {
		return nil
	}

	type resultado struct {
		id   string
		nome string
	}
	saida := make(chan resultado, len(ids))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			lead, err := h.crmClient.GetLead(ctx, tenantID, id)
			if err != nil || lead == nil {
				saida <- resultado{id: id}
				return
			}
			saida <- resultado{id: id, nome: lead.Name}
		}(id)
	}
	wg.Wait()
	close(saida)

	nomes := make(map[string]string, len(ids))
	for r := range saida {
		if strings.TrimSpace(r.nome) != "" {
			nomes[r.id] = r.nome
		}
	}
	if len(nomes) == 0 {
		return nil
	}
	return nomes
}

func internalChargesComAtraso(charges []*charge.Charge, nomes map[string]string) []internalChargeResponse {
	out := make([]internalChargeResponse, 0, len(charges))
	for _, c := range charges {
		if c == nil {
			continue
		}
		out = append(out, internalChargeResponse{
			Charge:      c,
			DaysOverdue: c.DaysOverdue(),
			DebtorName:  nomes[c.TenantPartyID],
		})
	}
	return out
}

// GetReceipt godoc
// @Summary      Download charge receipt PDF
// @Description  Devolve o recibo PDF da cobrança paga. 400 se status != PAID.
// @Tags         charges
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Success      200 {file} binary
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      404 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /charges/{id}/receipt [get]
func (h *ChargeHandler) GetReceipt(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	ch, err := h.service.GetByID(ctx, tenantID, c.Param("id"))
	if err != nil || ch == nil {
		response.NotFound(c, "charge not found")
		return
	}
	if ch.Status != charge.StatusPaid {
		response.ValidationError(c, "only PAID charges have a receipt")
		return
	}
	rctx := service.ChargeReceiptContext{Charge: ch}
	// Best-effort: o nome da imobiliária é enriquecimento. Se a leitura falhar,
	// o recibo ainda é emitido com o fallback neutro do renderer.
	if h.tenantCompany != nil {
		if name, lookupErr := h.tenantCompany.CompanyName(ctx, tenantID); lookupErr == nil {
			rctx.OrgName = name
		}
	}
	pdf, err := service.RenderChargeReceiptPDF(rctx)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.Header("Content-Disposition", "inline; filename=\"recibo-"+ch.ID+".pdf\"")
	c.Data(200, "application/pdf", pdf)
}

type ReconcileManualRequest struct {
	PaidAt        time.Time `json:"paid_at" binding:"required"`
	PaidAmount    float64   `json:"paid_amount" binding:"required,gt=0"`
	PaymentMethod string    `json:"payment_method" binding:"required"`
	ProofURL      string    `json:"proof_url"`
	Note          string    `json:"note"`
}

// CheckRepairRentalAllocations godoc
//
//	@Summary      Elegibilidade do reparo de alocação de locação
//	@Description  Read-only: informa se a cobrança de locação quitada tem algo a reparar (gate do botão no painel).
//	@Tags         charges
//	@Produce      json
//	@Param        id  path      string  true  "Charge ID"
//	@Success      200  {object}  response.Response
//	@Router       /charges/{id}/repair-rental-allocations/eligibility [get]
func (h *ChargeHandler) CheckRepairRentalAllocations(c *gin.Context) {
	ctx := c.Request.Context()
	chargeID := c.Param("id")
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	result, err := h.service.CheckRentalIntermediationRepairEligibility(ctx, tenantID, chargeID)
	if err != nil {
		if errors.Is(err, charge.ErrNotFound) || strings.Contains(err.Error(), "not found") {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Elegibilidade do reparo", result)
}

// RepairRentalAllocationsRequest é o corpo do reparo de alocações de locação.
type RepairRentalAllocationsRequest struct {
	// TenantID é aceito apenas em chamadas internas (credencial de serviço com
	// allowlist); no caminho com JWT o tenant vem do token e este campo é ignorado.
	TenantID string `json:"tenant_id,omitempty"`
	Reason   string `json:"reason" binding:"required"`
}

// RepairRentalAllocations godoc
//
//	@Summary      Repara alocações de uma cobrança de locação quitada antes do plano de intermediação
//	@Description  Re-deriva as alocações dos pagamentos sem a fatia de intermediação (financia comissão/payout),
//	@Description  estorna e regrava o lançamento de recebimento e reconcilia o repasse do proprietário.
//	@Tags         charges
//	@Accept       json
//	@Produce      json
//	@Param        id       path      string                          true  "Charge ID"
//	@Param        request  body      handlers.RepairRentalAllocationsRequest true "Repair"
//	@Success      200      {object}  response.Response
//	@Router       /charges/{id}/repair-rental-allocations [post]
func (h *ChargeHandler) RepairRentalAllocations(c *gin.Context) {
	chargeID := c.Param("id")
	if strings.TrimSpace(chargeID) == "" {
		response.ValidationError(c, "charge ID is required")
		return
	}
	var req RepairRentalAllocationsRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	// SECURITY (achado A3): o tenant vem da CREDENCIAL (JWT do usuário ou chave
	// de serviço com allowlist), nunca do chamador.
	tenantID, err := auth.ResolveTenantForInternalRequest(c, req.TenantID)
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
	operatorID, _ := context.GetAccountID(c.Request.Context())
	if strings.TrimSpace(operatorID) == "" {
		operatorID = "reparo-interno"
	}
	// O contexto precisa carregar o tenant: o pool de aplicação semeia as
	// claims de RLS a partir dele (ledger append-only e configs são lidos sob
	// RLS). A autorização já aconteceu acima, na resolução pela credencial.
	reqCtx := context.WithTenantID(c.Request.Context(), tenantID)

	result, err := h.service.RepairRentalIntermediationAllocations(reqCtx, tenantID, chargeID, operatorID, req.Reason)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Alocações reparadas", result)
}

// ReconcileManual godoc
// @Summary      Reconcile a charge off-platform (manual)
// @Description  Marca a charge como paga manualmente (sem gateway). Audita operador + proof.
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Param        request body handlers.ReconcileManualRequest true "Reconciliation"
// @Success      200 {object} response.Response{data=charge.Charge}
// @Router       /charges/{id}/reconcile-manual [post]
func (h *ChargeHandler) ReconcileManual(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.ReconcileManual")
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req ReconcileManualRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	idempotencyKey := strings.TrimSpace(getChargeIdempotencyKey(c))
	if idempotencyKey == "" {
		response.ValidationError(c, "Idempotency-Key header is required")
		return
	}
	chargeID := c.Param("id")
	paymentID := events.DeriveNotificationID("charge.payment", tenantID, chargeID, idempotencyKey)
	updated, err := h.service.MarkPaidIdempotent(ctx, tenantID, chargeID, req.PaymentMethod, req.PaidAmount, paymentID, req.PaidAt)
	if err != nil {
		log.Error("manual reconcile failed", "error", err)
		switch {
		case errors.Is(err, charge.ErrNotFound):
			response.NotFound(c, err.Error())
		case errors.Is(err, charge.ErrInvalidState), errors.Is(err, charge.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		default:
			respondInternalError(c, err)
		}
		return
	}
	// Operator-supplied evidence (proof_url, note, paid_at claim) é registrada
	// no audit trail mesmo sem coluna dedicada no charge — o ledger é a
	// fonte de verdade pra a reconciliação manual. ChargeService.MarkPaid já
	// gravou um "charge.paid"; este é o "charge.reconciled" complementar com
	// o que o operador disse versus o que o sistema gravou.
	if req.ProofURL != "" || req.Note != "" || !req.PaidAt.IsZero() {
		metadata := map[string]any{
			"path":           "ReconcileManual",
			"payment_method": req.PaymentMethod,
			"paid_amount":    req.PaidAmount,
		}
		if req.ProofURL != "" {
			metadata["proof_url"] = req.ProofURL
		}
		if req.Note != "" {
			metadata["note"] = req.Note
		}
		if !req.PaidAt.IsZero() {
			// PaidAt declarado pelo operador (e.g. data real do TED). O
			// MarkPaid usa time.Now() pra carimbar quando o sistema
			// reconheceu — guardamos a divergência no audit pra eventual
			// disputa.
			metadata["operator_paid_at"] = req.PaidAt
		}
		audit.Record(ctx, audit.Entry{
			Action:     "charge.reconciled",
			EntityType: "Charge",
			EntityID:   updated.ID,
			After:      map[string]any{"status": string(updated.Status)},
			Metadata:   metadata,
		})
	}
	response.Success(c, "Charge reconciled successfully", updated)
}

type CancelChargeRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// CancelCharge godoc
// @Summary      Cancel a charge
// @Description  Cancela uma cobrança PENDING/OVERDUE. Bloqueia PAID/CANCELLED com 409. Reason vai para o audit.
// @Tags         charges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Param        request body handlers.CancelChargeRequest true "Cancel"
// @Success      200 {object} response.Response{data=charge.Charge}
// @Failure      400 {object} response.Response "reason vazio"
// @Failure      404 {object} response.Response "charge não existe"
// @Failure      409 {object} response.Response "já paga ou já cancelada"
// @Router       /charges/{id}/cancel [post]
func (h *ChargeHandler) CancelCharge(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.CancelCharge", "charge_id", c.Param("id"))
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req CancelChargeRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		response.ValidationError(c, "reason is required")
		return
	}
	updated, err := h.service.Cancel(ctx, tenantID, c.Param("id"), strings.TrimSpace(req.Reason))
	if err != nil {
		switch {
		case errors.Is(err, charge.ErrChargeNotFound):
			response.NotFound(c, "charge not found")
		case errors.Is(err, charge.ErrChargeAlreadyPaid):
			response.Conflict(c, "cannot cancel a paid charge")
		case errors.Is(err, charge.ErrChargeAlreadyCancelled):
			response.Conflict(c, "charge is already cancelled")
		case errors.Is(err, charge.ErrInvalidState), errors.Is(err, charge.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		default:
			log.Error("cancel charge failed", "error", err)
			respondInternalError(c, err)
		}
		return
	}
	response.Success(c, "Charge cancelled successfully", updated)
}

// GeneratePaymentLink godoc
// @Summary      Generate payment link for a charge
// @Description  Cria o link de cobrança (Pix/boleto) da charge via provedor de pagamentos (P1-05). 409 quando o ambiente não tem provedor configurado.
// @Tags         charges
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Charge ID"
// @Success      200 {object} response.Response{data=handlers.PaymentLinkResponse}
// @Failure      404 {object} response.Response
// @Failure      409 {object} response.Response  "payment provider not configured"
// @Failure      500 {object} response.Response
// @Router       /charges/{id}/payment-link [post]
func (h *ChargeHandler) GeneratePaymentLink(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	link, err := h.service.GeneratePaymentLink(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentProviderNotConfigured):
			response.Conflict(c, "payment provider not configured")
		case err.Error() == "charge not found":
			response.NotFound(c, err.Error())
		case err.Error() == "charge is already paid":
			response.Conflict(c, err.Error())
		default:
			respondInternalError(c, err)
		}
		return
	}
	response.Success(c, "Payment link generated", PaymentLinkResponse{
		URL:        link.URL,
		ExternalID: link.ExternalID,
		Provider:   link.Provider,
		ExpiresAt:  link.ExpiresAt,
	})
}

// PaymentLinkResponse é o shape público do link de cobrança (P1-05).
type PaymentLinkResponse struct {
	URL        string    `json:"url"`
	ExternalID string    `json:"external_id"`
	Provider   string    `json:"provider"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Force usage of valueobjects package (already imported by other handlers; keeps
// the swagger generator happy when this file is the only consumer in a build
// variant).
var _ = valueobjects.Money{}

// ReverseChargeRequest — corpo de POST /charges/:id/reverse.
type ReverseChargeRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// ReverseCharge estorna uma cobrança PAGA (G30 do plano de inadimplência).
//
// O serviço existia desde sempre e não tinha rota: estornar uma cobrança paga
// era impossível pela API, então o dinheiro ficava onde estava e a dívida
// continuava quitada. A permissão é a de aprovação de repasse
// (`finance:approve_transfer`) porque o estorno mexe em valor já recebido e
// distribuído — mesma família do cancelamento e do reparo de alocação.
func (h *ChargeHandler) ReverseCharge(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "ChargeHandler.ReverseCharge", "charge_id", c.Param("id"))
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req ReverseChargeRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		response.ValidationError(c, "reason is required")
		return
	}
	updated, err := h.service.Reverse(ctx, tenantID, c.Param("id"), strings.TrimSpace(req.Reason))
	if err != nil {
		switch {
		case errors.Is(err, charge.ErrChargeNotFound):
			response.NotFound(c, "charge not found")
		case errors.Is(err, charge.ErrInvalidState), errors.Is(err, charge.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		default:
			log.Error("reverse charge failed", "error", err)
			respondInternalError(c, err)
		}
		return
	}
	response.Success(c, "Cobrança estornada", updated)
}

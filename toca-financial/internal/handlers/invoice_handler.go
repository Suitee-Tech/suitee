package handlers

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/invoice"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type InvoiceHandler struct {
	service *service.InvoiceService
}

func NewInvoiceHandler(service *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{service: service}
}

func getInvoiceIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := strings.TrimSpace(c.GetHeader("Idempotency-Key")); v != "" {
		return v
	}
	return strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
}

type CreateInvoiceRequest struct {
	ReferenceID string       `json:"reference_id" binding:"required"`
	ChargeID    string       `json:"charge_id" binding:"required"`
	PayerID     string       `json:"payer_id" binding:"required"`
	Amount      float64      `json:"amount" binding:"required,gt=0"`
	DueDate     time.Time    `json:"due_date" binding:"required"`
	Type        invoice.Type `json:"type" binding:"required"`
}

// Create godoc
// @Summary      Create invoice
// @Description  Create a new invoice
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        Idempotency-Key header string true "Stable invoice command key"
// @Param        request body CreateInvoiceRequest true "Invoice data"
// @Success      201  {object}  response.Response{data=invoice.Invoice}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      409  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /invoices [post]
func (h *InvoiceHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "InvoiceHandler.Create")

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	var req CreateInvoiceRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	idempotencyKey := getInvoiceIdempotencyKey(c)
	if idempotencyKey == "" {
		response.ValidationError(c, "Idempotency-Key header is required")
		return
	}
	// The header is the stable command identity. Payload equality is checked by
	// InvoiceService against the canonical row, so key reuse with changed data
	// becomes HTTP 409 instead of silently creating another invoice.
	sourceEventID := events.DeriveNotificationID(idempotencyKey, "invoice.create", tenantID)

	inv, err := h.service.CreateWithSourceAndCharge(ctx, req.ReferenceID, req.PayerID, req.Amount, req.DueDate, req.Type, tenantID, sourceEventID, req.ChargeID)
	if err != nil {
		log.Error("failed to create invoice", "error", err)
		switch {
		case errors.Is(err, charge.ErrNotFound):
			response.NotFound(c, err.Error())
		case errors.Is(err, service.ErrInvoiceIdempotencyConflict),
			errors.Is(err, service.ErrInvoiceChargeContractMismatch),
			errors.Is(err, service.ErrInvoiceChargePayerMismatch),
			errors.Is(err, service.ErrInvoiceAmountExceedsChargeBalance):
			response.Conflict(c, err.Error())
		case errors.Is(err, service.ErrInvoicePaymentApplicationUnavailable):
			response.InternalError(c, err)
		default:
			response.InternalServerError(c, "failed to create invoice")
		}
		return
	}

	log.Info("invoice created successfully", "invoice_id", inv.ID)
	response.Created(c, "Invoice created successfully", inv)
}

// List godoc
// @Summary      List invoices
// @Description  List invoices with optional status filter
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        status query string false "Status"
// @Param        reference_id query string false "Reference ID"
// @Success      200  {object}  response.Response{data=[]invoice.Invoice}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /invoices [get]
func (h *InvoiceHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "InvoiceHandler.List")

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "10"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

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
	issuedFrom, err := parseTime(c.Query("issued_from"))
	if err != nil {
		response.ValidationError(c, "invalid issued_from value")
		return
	}
	issuedTo, err := parseTime(c.Query("issued_to"))
	if err != nil {
		response.ValidationError(c, "invalid issued_to value")
		return
	}

	filter := invoice.SearchFilter{
		CustomerID:   c.Query("customer_id"),
		CustomerName: c.Query("customer_name"),
		Status:       c.Query("status"),
		MinAmount:    minAmount,
		MaxAmount:    maxAmount,
		IssuedFrom:   issuedFrom,
		IssuedTo:     issuedTo,
		ReferenceID:  c.Query("reference_id"),
		Limit:        limit,
		Offset:       offset,
	}

	invoices, total, err := h.service.Search(ctx, tenantID, filter)
	if err != nil {
		log.Error("failed to list invoices", "error", err)
		response.InternalServerError(c, "failed to list invoices")
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Invoices retrieved successfully", invoices, page, int(limit), total)
}

// GetByID godoc
// @Summary      Get invoice by ID
// @Description  Get an invoice by its ID
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Invoice ID"
// @Success      200  {object}  response.Response{data=invoice.Invoice}
// @Failure      404  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /invoices/{id} [get]
func (h *InvoiceHandler) GetByID(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	log := logger.FromContext(ctx).With("handler", "InvoiceHandler.GetByID", "invoice_id", id)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	inv, err := h.service.GetByID(ctx, tenantID, id)
	if err != nil {
		log.Warn("failed to get invoice", "error", err)
		if errors.Is(err, invoice.ErrInvoiceNotFound) {
			response.NotFound(c, err.Error())
		} else {
			response.InternalServerError(c, "failed to get invoice")
		}
		return
	}

	response.Success(c, "Invoice retrieved successfully", inv)
}

// Pay godoc
// @Summary      Pay invoice
// @Description  Mark an invoice as paid
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Invoice ID"
// @Param        Idempotency-Key header string true "Stable invoice payment key"
// @Success      200  {object}  response.Response{data=invoice.Invoice}
// @Failure      400  {object}  response.Response
// @Failure      409  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /invoices/{id}/pay [post]
func (h *InvoiceHandler) Pay(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	log := logger.FromContext(ctx).With("handler", "InvoiceHandler.Pay", "invoice_id", id)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		log.Warn("tenant context missing")
		response.Forbidden(c, "tenant context required")
		return
	}

	idempotencyKey := getInvoiceIdempotencyKey(c)
	if idempotencyKey == "" {
		response.ValidationError(c, "Idempotency-Key header is required")
		return
	}
	paymentID := events.DeriveNotificationID("invoice.payment", tenantID, id, idempotencyKey)
	inv, err := h.service.PayIdempotent(ctx, tenantID, id, paymentID, "INVOICE", time.Now())
	if err != nil {
		log.Warn("failed to pay invoice", "error", err)
		switch {
		case errors.Is(err, invoice.ErrInvoiceNotFound), errors.Is(err, charge.ErrNotFound):
			response.NotFound(c, err.Error())
		case errors.Is(err, service.ErrInvoiceChargeLinkRequired),
			errors.Is(err, service.ErrInvoiceChargeContractMismatch),
			errors.Is(err, service.ErrInvoiceChargePayerMismatch),
			errors.Is(err, service.ErrInvoiceAmountExceedsChargeBalance),
			errors.Is(err, charge.ErrInvalidState),
			errors.Is(err, charge.ErrConcurrentModification),
			err.Error() == "cannot pay a cancelled invoice":
			response.Conflict(c, err.Error())
		case errors.Is(err, service.ErrInvoicePaymentApplicationUnavailable):
			response.InternalError(c, err)
		default:
			response.InternalServerError(c, "failed to process payment")
		}
		return
	}

	log.Info("invoice paid successfully")
	response.Success(c, "Invoice paid successfully", inv)
}

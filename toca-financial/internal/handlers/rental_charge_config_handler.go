package handlers

import (
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

var _ = rental_charge_config.RentalChargeConfig{}

type RentalChargeConfigHandler struct {
	service *service.RentalChargeConfigService
}

func NewRentalChargeConfigHandler(service *service.RentalChargeConfigService) *RentalChargeConfigHandler {
	return &RentalChargeConfigHandler{service: service}
}

type CreateRentalChargeConfigRequest struct {
	ContractID      string    `json:"contract_id" binding:"required"`
	TenantPartyID   string    `json:"tenant_party_id" binding:"required"`
	OwnerID         string    `json:"owner_id" binding:"required"`
	PropertyID      string    `json:"property_id" binding:"required"`
	StartDate       time.Time `json:"start_date" binding:"required"`
	EndDate         time.Time `json:"end_date" binding:"required"`
	FirstChargeDate time.Time `json:"first_charge_date" binding:"required"`
	FirstChargeType string    `json:"first_charge_type" binding:"required"`
	DefaultDueDay   int       `json:"default_due_day" binding:"required,min=1,max=31"`
	MonthlyAmount   float64   `json:"monthly_amount" binding:"required,gt=0"`
}

type UpdateRentalChargeConfigRequest struct {
	FirstChargeDate time.Time `json:"first_charge_date" binding:"required"`
	DefaultDueDay   int       `json:"default_due_day" binding:"required,min=1,max=31"`
}

type CreateRentalChargeConfigResponse struct {
	Config         *rental_charge_config.RentalChargeConfig `json:"config"`
	ChargesCreated int                                      `json:"charges_created"`
}

type GenerateChargesResponse struct {
	ChargesCreated int `json:"charges_created"`
}

func (h *RentalChargeConfigHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx).With("handler", "RentalChargeConfigHandler.Create")

	var req CreateRentalChargeConfigRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	monthlyCents := int64(math.Round(req.MonthlyAmount * 100))
	// A emissão de charges RENT é uma autoridade única e explícita: este
	// endpoint persiste apenas o cronograma (config). Nenhuma cobrança é
	// criada antes da assinatura; a materialização acontece depois via
	// POST /rental-charge-configs/:id/generate-charges.
	cfg, err := h.service.CreateConfigOnly(
		ctx,
		req.ContractID,
		tenantID,
		req.TenantPartyID,
		req.OwnerID,
		req.PropertyID,
		req.StartDate,
		req.EndDate,
		req.FirstChargeDate,
		rental_charge_config.FirstChargeType(req.FirstChargeType),
		req.DefaultDueDay,
		monthlyCents,
	)
	if err != nil {
		log.Error("failed to create rental charge config", "error", err)
		respondInternalError(c, err)
		return
	}

	response.Created(c, "Rental charge config created", CreateRentalChargeConfigResponse{
		Config:         cfg,
		ChargesCreated: 0,
	})
}

func (h *RentalChargeConfigHandler) GetByID(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	log := logger.FromContext(ctx).With("handler", "RentalChargeConfigHandler.GetByID", "id", id)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	cfg, err := h.service.GetByID(ctx, tenantID, id)
	if err != nil {
		log.Error("failed to get rental charge config", "error", err)
		respondInternalError(c, err)
		return
	}
	if cfg == nil {
		response.NotFound(c, "rental charge config not found")
		return
	}

	response.Success(c, "Rental charge config retrieved", cfg)
}

// GetByContractID powers the admin "Configuração de Cobranças" panel which
// needs to find the existing config (if any) for a given contract before
// deciding whether to render the create form or the update form.
//
// "No config for this contract" is an expected, legitimate answer (most
// contracts have none until a pacote is set up), so it returns 200 with a
// null data payload rather than 404. A 404 here would be misleading — the
// endpoint exists — and it made both the browser and the frontend API
// interceptor log a red "error" for the normal first-time-setup path. The
// caller branches on data == null exactly as it did on the old 404.
func (h *RentalChargeConfigHandler) GetByContractID(c *gin.Context) {
	ctx := c.Request.Context()
	contractID := c.Param("contract_id")
	log := logger.FromContext(ctx).With("handler", "RentalChargeConfigHandler.GetByContractID", "contract_id", contractID)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	cfg, err := h.service.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		// The repo wraps sql.ErrNoRows when no row matched — that's the "not
		// configured yet" case, returned as 200/null above. Everything else is
		// a genuine failure: surface timeouts as 504 (so "too slow" is
		// distinguishable from "broken") and anything else as 500. The previous
		// implementation masked ALL of these as 404, hiding real outages.
		if errors.Is(err, sql.ErrNoRows) {
			response.Success(c, "no rental charge config for contract", nil)
			return
		}
		if database.IsTimeout(err) {
			log.Warn("rental charge config lookup timed out", "error", err)
			response.GatewayTimeout(c, "request timeout while loading rental charge config")
			return
		}
		log.Error("failed to get rental charge config", "error", err)
		respondInternalError(c, err)
		return
	}
	if cfg == nil {
		response.Success(c, "no rental charge config for contract", nil)
		return
	}

	response.Success(c, "Rental charge config retrieved", cfg)
}

func (h *RentalChargeConfigHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	log := logger.FromContext(ctx).With("handler", "RentalChargeConfigHandler.Update", "id", id)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateRentalChargeConfigRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	cfg, err := h.service.UpdateConfig(ctx, tenantID, id, req.FirstChargeDate, req.DefaultDueDay)
	if err != nil {
		if err.Error() == "first charge already paid" {
			response.Forbidden(c, err.Error())
			return
		}
		if err.Error() == "rental charge config not found" {
			response.NotFound(c, err.Error())
			return
		}
		log.Error("failed to update rental charge config", "error", err)
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Rental charge config updated", cfg)
}

// UpdateBundledItemsRequest is the body of POST
// /rental-charge-configs/:id/bundled-items. The whole BundledItems slice
// is replaced (PATCH-style merging adds complexity for no UX gain — the
// admin UI always sends the full list).
type UpdateBundledItemsRequest struct {
	Items []BundledItemPayload `json:"items"`
}

type BundledItemPayload struct {
	Key         string `json:"key" binding:"required"`
	Description string `json:"description" binding:"required"`
	AmountCents int64  `json:"amount_cents" binding:"required,gt=0"`
	Enabled     bool   `json:"enabled"`
}

// UpdateBundledItems replaces the bundled items of an existing rental
// charge config and regenerates every PENDING future charge with the new
// total + line items. Charges already PAID/PARTIALLY_PAID are untouched —
// past months stay billed at the old value, only the future calendar
// follows the new pacote.
func (h *RentalChargeConfigHandler) UpdateBundledItems(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req UpdateBundledItemsRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	items := make([]rental_charge_config.BundledItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, rental_charge_config.BundledItem{
			Key:         it.Key,
			Description: it.Description,
			AmountCents: it.AmountCents,
			Enabled:     it.Enabled,
		})
	}
	cfg, created, err := h.service.UpdateBundledItems(ctx, tenantID, id, items)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Bundled items updated", gin.H{"config": cfg, "charges_created": created})
}

func (h *RentalChargeConfigHandler) GenerateCharges(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	log := logger.FromContext(ctx).With("handler", "RentalChargeConfigHandler.GenerateCharges", "id", id)

	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	created, err := h.service.GenerateChargesByID(ctx, tenantID, id)
	if err != nil {
		if err.Error() == "rental charge config not found" {
			response.NotFound(c, err.Error())
			return
		}
		log.Error("failed to generate charges", "error", err)
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Charges generated", GenerateChargesResponse{ChargesCreated: created})
}

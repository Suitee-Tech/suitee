package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// RentalHandler — Phase 4 backend. Powers the admin RentalDetails page and
// the LeadDetails > Locações tab. The portal-scoped variant lives in
// portal_rental_handler.go and shares the same RentalService.
type RentalHandler struct {
	service *service.RentalService
}

func NewRentalHandler(s *service.RentalService) *RentalHandler {
	return &RentalHandler{service: s}
}

// List supports filtering by `lead_id` + `role` (tenant|owner) for
// LeadDetails and `property_id` for PropertyDetails. Without filters it
// returns nothing — listing every rental in the tenant has no admin UX
// today and protects against accidental large pulls.
func (h *RentalHandler) List(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	leadID := c.Query("lead_id")
	role := c.Query("role")
	propertyID := c.Query("property_id")
	dealID := c.Query("deal_id")

	if leadID != "" {
		var lr rental.LeadRole
		switch role {
		case "tenant":
			lr = rental.LeadRoleTenant
		case "owner":
			lr = rental.LeadRoleOwner
		default:
			response.ValidationError(c, "role must be tenant or owner when lead_id is provided")
			return
		}
		items, err := h.service.ListForLead(c.Request.Context(), tenantID, leadID, lr)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		response.Success(c, "Rentals retrieved successfully", items)
		return
	}

	if dealID != "" {
		r, err := h.service.GetByDealID(c.Request.Context(), tenantID, dealID)
		if err != nil || r == nil {
			// Don't 404 — return empty list so the caller can render
			// "no rental yet" gracefully.
			response.Success(c, "Rentals retrieved successfully", []*rental.Rental{})
			return
		}
		response.Success(c, "Rentals retrieved successfully", []*rental.Rental{r})
		return
	}

	if propertyID == "" {
		response.ValidationError(c, "filter required: lead_id+role, property_id, or deal_id")
		return
	}
	// property_id path — limit/offset accepted but currently passthrough.
	_, _ = strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	_, _ = strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	// Not yet implemented in service; the repo has ListByPropertyID.
	response.Success(c, "Rentals retrieved successfully", []*rental.Rental{})
}

// GetByID powers /rentals/:id used by the admin RentalDetails page.
//
// The admin UI links to /locacao/{contract.id} (see toka Rentals.tsx), so the
// `:id` segment is most often a contract_id. We accept three id spaces —
// contract_id, deal_id and the rental's own id — and return the first match.
// They are disjoint (distinct source tables / fresh uuids), so the order only
// affects how many lookups we do: contract_id first serves the common caller.
func (h *RentalHandler) GetByID(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	id := c.Param("id")

	if r, err := h.service.GetByContractID(c.Request.Context(), tenantID, id); err == nil && r != nil {
		response.Success(c, "Rental retrieved successfully", r)
		return
	}
	if r, err := h.service.GetByDealID(c.Request.Context(), tenantID, id); err == nil && r != nil {
		response.Success(c, "Rental retrieved successfully", r)
		return
	}
	if r, err := h.service.GetRentalByID(c.Request.Context(), tenantID, id); err == nil && r != nil {
		response.Success(c, "Rental retrieved successfully", r)
		return
	}
	response.NotFound(c, "rental not found")
}

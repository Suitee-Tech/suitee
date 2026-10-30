package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// PortalRentalHandler exposes /client-portal/rentals — Phase 5.1.
//
// Resolution flow:
//  1. Read tenant_id + account_id from JWT context.
//  2. Use crossRepo.LeadIDsForAccount to find every lead.id that has
//     metadata.account_id = caller's account_id.
//  3. Query rentals where any of those lead_ids is tenant_lead_id or
//     owner_lead_id. Returns a flat list — the frontend distinguishes
//     "tenant of" vs "owner of" by comparing each rental's IDs against
//     the set returned in step 2.
//
// Why not pass `lead_id=me` to the existing /rentals admin endpoint:
// admin-scope queries require `rentals:read` permission which clients
// don't have. The portal-scoped endpoint sits under client-portal/* which
// is gated by `RequireFunctionalRole("cliente")` instead.
type PortalRentalHandler struct {
	rentalService *service.RentalService
	crossRepo     repository.CrossTenantRepository
}

func NewPortalRentalHandler(rentalService *service.RentalService, crossRepo repository.CrossTenantRepository) *PortalRentalHandler {
	return &PortalRentalHandler{rentalService: rentalService, crossRepo: crossRepo}
}

// PortalRentalDTO mirrors RentalDTO but adds a "my_role" hint so the
// frontend doesn't need to recompute it from tenant_lead_id/owner_lead_id.
type PortalRentalDTO struct {
	Rental *rental.Rental `json:"rental"`
	MyRole string         `json:"my_role"` // "tenant" / "owner" / "both"
}

// ListMine handles GET /api/v1/client-portal/rentals.
func (h *PortalRentalHandler) ListMine(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	accountID, _ := context.GetAccountID(c.Request.Context())
	if accountID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	leadIDs, err := h.crossRepo.LeadIDsForAccount(c.Request.Context(), tenantID, accountID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if len(leadIDs) == 0 {
		response.Success(c, "Rentals retrieved successfully", []PortalRentalDTO{})
		return
	}

	rentals, err := h.rentalService.ListForLeadIDs(c.Request.Context(), tenantID, leadIDs)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	leadSet := make(map[string]bool, len(leadIDs))
	for _, id := range leadIDs {
		leadSet[id] = true
	}

	out := make([]PortalRentalDTO, 0, len(rentals))
	for _, r := range rentals {
		role := ""
		isTenant := leadSet[r.TenantLeadID]
		isOwner := leadSet[r.OwnerLeadID]
		switch {
		case isTenant && isOwner:
			role = "both"
		case isTenant:
			role = "tenant"
		case isOwner:
			role = "owner"
		}
		out = append(out, PortalRentalDTO{Rental: r, MyRole: role})
	}

	response.Success(c, "Rentals retrieved successfully", out)
}

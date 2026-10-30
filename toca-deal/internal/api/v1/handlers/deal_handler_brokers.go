package handlers

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// Broker participation endpoints own split validation and audit attribution.

type AddBrokerParticipationRequest struct {
	BrokerID   string                       `json:"broker_id" binding:"required"`
	Role       deal.BrokerParticipationRole `json:"role" binding:"required,oneof=captador co_captador visitador atendente fechamento imobiliaria"`
	Commission float64                      `json:"commission,omitempty"`
	Percentage float64                      `json:"percentage,omitempty"`
}

// AddBrokerParticipation godoc
// @Summary      Add broker participation to deal
// @Description  Add a broker with a specific role (captador, visitador, fechamento) to a deal for history and commission purposes
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.AddBrokerParticipationRequest true "Add Broker Participation Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/broker-participations [post]
func (h *DealHandler) AddBrokerParticipation(c *gin.Context) {
	id := c.Param("id")
	var req AddBrokerParticipationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	scopeBrokerID, _, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	d, err := h.service.AddBrokerParticipation(c.Request.Context(), tenantID, id, scopeBrokerID, req.BrokerID, req.Role, req.Commission, req.Percentage)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Broker participation added successfully", d)
}

// SetBrokerParticipationsRequest carries the full replacement payload for
// PUT /deals/:id/broker-participations. Used by admin/gestor UI to adjust
// the commission distribution (captador / visitador / fechamento) — issue
// #420. Sum of percentages must be 100% (±0.01); validated server-side.
// Commission optionally updates the deal-level commission percentage.
type SetBrokerParticipationsRequest struct {
	Commission     *float64                     `json:"commission,omitempty" binding:"omitempty,gt=0,lte=100"`
	Participations []SetBrokerParticipationItem `json:"participations" binding:"required,min=1,dive"`
}

type SetBrokerParticipationItem struct {
	BrokerID   string                       `json:"broker_id"`
	Role       deal.BrokerParticipationRole `json:"role" binding:"required,oneof=captador co_captador visitador atendente fechamento imobiliaria"`
	Commission float64                      `json:"commission,omitempty"`
	Percentage float64                      `json:"percentage" binding:"min=0,max=100"`
}

// SetBrokerParticipations godoc
// @Summary      Replace all broker participations on a deal
// @Description  Replace the entire broker_participations array. Sum of percentages must be 100% (±0.01). Locked once the deal reaches CONVERTED (contract signed). Issue #420.
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.SetBrokerParticipationsRequest true "Broker Participations payload"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      409  {object}  response.Response "Deal is locked (CONVERTED/CANCELLED)"
// @Failure      422  {object}  response.Response "Sum is not 100"
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/broker-participations [put]
func (h *DealHandler) SetBrokerParticipations(c *gin.Context) {
	id := c.Param("id")
	var req SetBrokerParticipationsRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	scopeBrokerID, _, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	// Actor identification for audit. Both ID and name are best-effort —
	// scopedBrokerID lookup already authorises the call; here we just
	// enrich the audit trail. We pass the name (not email) because that's
	// what `pkg/context` exposes today; audit consumers (admin UI) prefer
	// the human-readable label anyway.
	actorID, _ := context.GetAccountID(c.Request.Context())
	actorEmail, _ := context.GetUserName(c.Request.Context())

	// Translate the request DTO into the domain type once so the service
	// signature stays tight. defensive copy: the user can't mutate the
	// slice we hold via shared pointer.
	domainBPs := make([]deal.BrokerParticipation, 0, len(req.Participations))
	for _, item := range req.Participations {
		domainBPs = append(domainBPs, deal.BrokerParticipation{
			BrokerID:   item.BrokerID,
			Role:       item.Role,
			Commission: item.Commission,
			Percentage: item.Percentage,
		})
	}

	d, err := h.service.SetBrokerParticipations(c.Request.Context(), tenantID, id, scopeBrokerID, actorID, actorEmail, req.Commission, domainBPs)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidSplitSum), errors.Is(err, service.ErrInvalidCommissionPercentage):
			// 422 Unprocessable — caller's payload was syntactically valid
			// but business-invalid (sum ≠ 100, percentage out of range).
			response.UnprocessableEntity(c, err.Error())
			return
		case errors.Is(err, service.ErrBrokerNotInTenant):
			// N14: id de corretor que não existe nesta imobiliária. 422 pelo
			// mesmo critério acima — payload bem formado, conteúdo inválido.
			response.UnprocessableEntity(c, err.Error())
			return
		case errors.Is(err, service.ErrSplitLockedAfterSigned):
			// 409 Conflict — request collides with the deal lifecycle.
			// Frontend already shows the lock badge but it's race-safe
			// to enforce server-side: two admins on two tabs could try
			// to edit while a contract just landed in SIGNED.
			response.Conflict(c, err.Error())
			return
		default:
			response.InternalError(c, err)
			return
		}
	}

	response.Success(c, "Broker participations updated successfully", d)
}

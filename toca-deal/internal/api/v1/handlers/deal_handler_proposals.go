package handlers

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// Proposal endpoints are grouped independently from the core deal lifecycle.

type AddProposalRequest struct {
	Value         float64 `json:"value" binding:"required"`
	PaymentMethod string  `json:"payment_method" binding:"required"`
	Installments  int     `json:"installments"`
	DownPayment   float64 `json:"down_payment"`
	Notes         string  `json:"notes"`
}

// AddProposal godoc
// @Summary      Add proposal
// @Description  Add a new proposal to a deal
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.AddProposalRequest true "Add Proposal Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/proposals [post]
func (h *DealHandler) AddProposal(c *gin.Context) {
	id := c.Param("id")
	var req AddProposalRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	d, err := h.service.AddProposal(c.Request.Context(), tenantID, id, brokerID, req.Value, req.PaymentMethod, req.Installments, req.DownPayment, userID, req.Notes)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Proposal added successfully", d)
}

type RejectProposalRequest struct {
	Reason       string   `json:"reason"`
	CounterValue *float64 `json:"counter_value,omitempty"`
}

func (h *DealHandler) AcceptMyDealProposal(c *gin.Context) {
	id := c.Param("id")
	proposalID := c.Param("proposalId")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())

	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	d, err := h.service.AcceptMyDealProposal(c.Request.Context(), accountID, email, id, proposalID)
	if err != nil {
		if err.Error() == "party not found" || err.Error() == "only primary parties can accept proposals" || err.Error() == "party role cannot accept proposals" {
			response.Forbidden(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Proposal accepted successfully", d)
}

func (h *DealHandler) RejectMyDealProposal(c *gin.Context) {
	id := c.Param("id")
	proposalID := c.Param("proposalId")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())

	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req RejectProposalRequest
	if !validation.BindJSONOptional(c, &req) {
		return
	}

	d, err := h.service.RejectMyDealProposal(c.Request.Context(), accountID, email, id, proposalID, req.Reason, req.CounterValue)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Proposal rejected successfully", d)
}

// AcceptCounterOfferRequest é o body do endpoint admin para aceitar
// contrapropostas (#424). proposal_id é obrigatório porque um deal pode
// ter várias propostas ao longo do tempo — não dá pra inferir "a última"
// sem risco de aceitar a errada.
type AcceptCounterOfferRequest struct {
	ProposalID string `json:"proposal_id" binding:"required"`
}

// AcceptCounterOffer godoc
//
// @Summary      Accept counter-offer (admin)
// @Description  Endpoint admin/operador para aceitar uma contraproposta enviada pelo cliente. Deal.Value é atualizado para o CounterValue da proposal, status vira ACCEPTED, evento publicado.
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.AcceptCounterOfferRequest true "Counter-offer acceptance"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response  "proposal_id required"
// @Failure      403  {object}  response.Response  "tenant context required"
// @Failure      404  {object}  response.Response  "deal or proposal not found"
// @Failure      409  {object}  response.Response  "proposal is not in COUNTER_OFFER status"
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/accept-counter-offer [post]
func (h *DealHandler) AcceptCounterOffer(c *gin.Context) {
	id := c.Param("id")
	var req AcceptCounterOfferRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// Actor (admin/gestor) — capturamos do JWT para audit. UserName é
	// best-effort (vem do JWT name claim quando presente) e serve como
	// rótulo human-readable no audit; o accountID é a chave forense
	// canônica.
	actorID, _ := context.GetAccountID(c.Request.Context())
	actorLabel, _ := context.GetUserName(c.Request.Context())

	d, err := h.service.AcceptCounterOffer(c.Request.Context(), tenantID, id, req.ProposalID, actorID, actorLabel)
	if err != nil {
		msg := err.Error()
		switch {
		case errors.Is(err, service.ErrProposalNotInCounterOffer):
			response.Conflict(c, msg)
			return
		case msg == "deal not found" || msg == "proposal not found":
			response.NotFound(c, msg)
			return
		}
		response.InternalServerError(c, msg)
		return
	}

	response.Success(c, "Counter-offer accepted successfully", d)
}

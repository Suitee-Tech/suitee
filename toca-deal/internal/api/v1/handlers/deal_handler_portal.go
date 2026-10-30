package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// maskPortalEmail mantém o e-mail reconhecível no log sem expor o endereço
// inteiro (o portal é uma superfície de cliente final).
func maskPortalEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return ""
	}
	local, domain := email[:at], email[at+1:]
	visible := local
	if len(visible) > 2 {
		visible = visible[:2]
	}
	return visible + "***@" + domain
}

// Authenticated client portal deal views, channels, favorites and signature
// projection live together with their PII-filtering helpers.

type ClientPortalDealDTO struct {
	*deal.Deal
	MyPartyID string         `json:"my_party_id,omitempty"`
	MyRole    deal.PartyRole `json:"my_role,omitempty"`
}

func findMyParty(d *deal.Deal, accountID, email string) *deal.Party {
	if d == nil {
		return nil
	}

	if accountID != "" {
		for i := range d.Parties {
			if d.Parties[i].AccountID == accountID {
				return &d.Parties[i]
			}
		}
	}
	if email != "" {
		for i := range d.Parties {
			if strings.EqualFold(d.Parties[i].Email, email) {
				return &d.Parties[i]
			}
		}
	}

	if accountID != "" || email != "" {
		for _, s := range d.Signers {
			if accountID != "" && s.AccountID == accountID {
				for i := range d.Parties {
					if d.Parties[i].ID == s.ID {
						return &d.Parties[i]
					}
				}
			}
			if email != "" && strings.EqualFold(s.Email, email) {
				for i := range d.Parties {
					if d.Parties[i].ID == s.ID {
						return &d.Parties[i]
					}
				}
			}
		}
	}

	return nil
}

func dealForPortal(d *deal.Deal, my *deal.Party) *deal.Deal {
	if d == nil {
		return nil
	}
	copyDeal := *d
	if my == nil {
		copyDeal.Parties = nil
		return &copyDeal
	}

	parties := make([]deal.Party, 0, 3)
	myCopy := *my
	parties = append(parties, myCopy)

	customerRole := deal.PartyRoleTenant
	if copyDeal.Type == deal.TypeSale {
		customerRole = deal.PartyRoleBuyer
	}

	addIfNotExists := func(p *deal.Party) {
		if p == nil {
			return
		}
		for i := range parties {
			if parties[i].ID == p.ID {
				return
			}
		}
		parties = append(parties, *p)
	}

	switch my.Role {
	case deal.PartyRoleOwner:
		addIfNotExists(copyDeal.PrimaryByRole(customerRole))
	case deal.PartyRoleTenant, deal.PartyRoleBuyer:
		addIfNotExists(copyDeal.PrimaryByRole(deal.PartyRoleOwner))
	default:
		addIfNotExists(copyDeal.PrimaryByRole(customerRole))
		addIfNotExists(copyDeal.PrimaryByRole(deal.PartyRoleOwner))
	}

	copyDeal.Parties = parties
	return &copyDeal
}

// ── Client Portal Handlers ──

// GetMyLeads godoc
// @Summary      Get client's leads
// @Description  Get all leads/interests where the user is the client
// @Tags         client-portal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/leads [get]
func (h *DealHandler) GetMyLeads(c *gin.Context) {
	accountID, err := context.GetAccountID(c.Request.Context())
	if err != nil || accountID == "" {
		response.Forbidden(c, "user context required")
		return
	}

	leads, err := h.service.GetMyLeads(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Client leads retrieved successfully", leads)
}

// GetMyDeals godoc
// @Summary      Get my deals
// @Description  List deals where the current user is a signer
// @Tags         portal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]deal.Deal}
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/deals [get]
func (h *DealHandler) GetMyDeals(c *gin.Context) {
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	deals, err := h.service.ListBySignerIdentity(c.Request.Context(), accountID, email)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	out := make([]ClientPortalDealDTO, 0, len(deals))
	for _, d := range deals {
		my := findMyParty(d, accountID, email)
		myID := ""
		myRole := deal.PartyRole("")
		if my != nil {
			myID = my.ID
			myRole = my.Role
		}
		out = append(out, ClientPortalDealDTO{
			Deal:      dealForPortal(d, my),
			MyPartyID: myID,
			MyRole:    myRole,
		})
	}

	if len(out) == 0 {
		// Sintoma de suporte: lista vazia significa que nenhum negócio casou a
		// identidade do token (account_id/e-mail do JWT × metadata->'signers').
		// Registrar a identidade USADA evita repetir a caçada (RLS × roteamento
		// × cadastro) a cada relato de "não aparece no portal".
		logger.Warn("client portal deals resolved empty",
			"account_id", accountID,
			"email", maskPortalEmail(email),
			"has_account", accountID != "",
			"has_email", email != "",
			"service_tx", database.ServiceTxFromCtx(c.Request.Context()))
	}

	response.Success(c, "Deals retrieved successfully", out)
}

func (h *DealHandler) ListMyChannels(c *gin.Context) {
	accountID, _ := context.GetAccountID(c.Request.Context())

	if accountID == "" {
		response.Unauthorized(c, "account authentication required")
		return
	}

	channels, err := h.service.ListMyChannels(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Channels retrieved successfully", channels)
}

func (h *DealHandler) GetMyDealChannel(c *gin.Context) {
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	dealID := c.Param("id")

	if accountID == "" && email == "" {
		response.Unauthorized(c, "authentication required")
		return
	}

	channel, err := h.service.GetMyDealChannel(c.Request.Context(), accountID, email, dealID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Channel retrieved successfully", channel)
}

// GetMyDeal godoc
// @Summary      Get my deal
// @Description  Get a deal by ID where the current user is a signer
// @Tags         portal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /client-portal/deals/{id} [get]
func (h *DealHandler) GetMyDeal(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	deals, err := h.service.ListBySignerIdentity(c.Request.Context(), accountID, email)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	var foundDeal *deal.Deal
	for _, d := range deals {
		if d.ID == id {
			foundDeal = d
			break
		}
	}

	if foundDeal == nil {
		response.Forbidden(c, "deal not found or access denied")
		return
	}

	my := findMyParty(foundDeal, accountID, email)
	myID := ""
	myRole := deal.PartyRole("")
	if my != nil {
		myID = my.ID
		myRole = my.Role
	}

	response.Success(c, "Deal retrieved successfully", ClientPortalDealDTO{
		Deal:      dealForPortal(foundDeal, my),
		MyPartyID: myID,
		MyRole:    myRole,
	})
}

type AddFavoriteRequest struct {
	PropertyID string `json:"property_id" binding:"required"`
}

func (h *DealHandler) GetMyDealFavorites(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	ids, err := h.service.ListMyDealFavorites(c.Request.Context(), accountID, email, id)
	if err != nil {
		if err.Error() == "deal not found or access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Favorites retrieved successfully", ids)
}

func (h *DealHandler) AddMyDealFavorite(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req AddFavoriteRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	d, err := h.service.AddMyDealFavorite(c.Request.Context(), accountID, email, id, req.PropertyID)
	if err != nil {
		if err.Error() == "deal not found or access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		if err.Error() == "property_id is required" {
			response.ValidationError(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Favorite added successfully", d)
}

func (h *DealHandler) RemoveMyDealFavorite(c *gin.Context) {
	id := c.Param("id")
	propertyID := c.Param("propertyId")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	d, err := h.service.RemoveMyDealFavorite(c.Request.Context(), accountID, email, id, propertyID)
	if err != nil {
		if err.Error() == "deal not found or access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		if err.Error() == "property_id is required" {
			response.ValidationError(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Favorite removed successfully", d)
}

func (h *DealHandler) GetMyDealProperty(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	p, err := h.service.GetMyDealProperty(c.Request.Context(), accountID, email, id)
	if err != nil {
		respondPortalError(c, err)
		return
	}
	response.Success(c, "Property retrieved successfully", p)
}

// ClientPortalSignatureDTO is the per-deal view of the signature pipeline
// shown to the portal client. Other signers are PII-filtered (name + role
// + status only) — email/phone/document never leak through this endpoint.
type ClientPortalSignerDTO struct {
	SignerID string `json:"signer_id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	SignedAt string `json:"signed_at,omitempty"`
	// RequestSignatureKey is the per-signer Clicksign widget token. Only
	// populated for the calling user's record (MySigner) — never leaks
	// for OtherSigners.
	RequestSignatureKey string `json:"request_signature_key,omitempty"`
}

type ClientPortalSignatureDTO struct {
	HasEnvelope    bool                    `json:"has_envelope"`
	ContractID     string                  `json:"contract_id,omitempty"`
	EnvelopeID     string                  `json:"envelope_id,omitempty"`
	EnvelopeStatus string                  `json:"envelope_status,omitempty"`
	Environment    string                  `json:"environment,omitempty"`
	Deadline       string                  `json:"deadline,omitempty"`
	SignedDocURL   string                  `json:"signed_document_url,omitempty"`
	MySigner       *ClientPortalSignerDTO  `json:"my_signer,omitempty"`
	OtherSigners   []ClientPortalSignerDTO `json:"other_signers"`
	SignedCount    int                     `json:"signed_count"`
	TotalCount     int                     `json:"total_count"`
}

// GetMyDealSignature godoc
// @Summary      Signature status for the calling portal user
// @Description  Returns envelope status + the user's own signer record + a
// @Description  PII-filtered list of the other signers (name, role, status).
// @Tags         portal
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Success      200  {object}  response.Response{data=ClientPortalSignatureDTO}
// @Router       /client-portal/deals/{id}/signature [get]
func (h *DealHandler) GetMyDealSignature(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	if h.contractService == nil {
		response.InternalServerError(c, "signature view not wired")
		return
	}

	// Access check piggybacks on the existing portal pattern: the caller
	// must appear as a signer on the deal. ListBySignerIdentity already
	// filters by accountID/email.
	deals, err := h.service.ListBySignerIdentity(c.Request.Context(), accountID, email)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	var found *deal.Deal
	for _, d := range deals {
		if d.ID == id {
			found = d
			break
		}
	}
	if found == nil {
		response.Forbidden(c, "deal not found or access denied")
		return
	}

	view, err := h.contractService.GetPortalSignatureView(c.Request.Context(), found.TenantID, id, email)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	dto := ClientPortalSignatureDTO{
		HasEnvelope:    view.HasEnvelope,
		ContractID:     view.ContractID,
		EnvelopeID:     view.EnvelopeID,
		EnvelopeStatus: view.EnvelopeStatus,
		Environment:    view.Environment,
		Deadline:       view.Deadline,
		SignedDocURL:   view.SignedDocURL,
		SignedCount:    view.SignedCount,
		TotalCount:     view.TotalCount,
		OtherSigners:   make([]ClientPortalSignerDTO, 0, len(view.OtherSigners)),
	}
	if view.MySigner != nil {
		dto.MySigner = &ClientPortalSignerDTO{
			SignerID:            view.MySigner.SignerID,
			Name:                view.MySigner.Name,
			Role:                view.MySigner.Role,
			Status:              view.MySigner.Status,
			SignedAt:            view.MySigner.SignedAt,
			RequestSignatureKey: view.MySigner.RequestSignatureKey,
		}
	}
	for _, s := range view.OtherSigners {
		dto.OtherSigners = append(dto.OtherSigners, ClientPortalSignerDTO{
			SignerID: s.SignerID,
			Name:     s.Name,
			Role:     s.Role,
			Status:   s.Status,
			SignedAt: s.SignedAt,
		})
	}
	response.Success(c, "Signature view retrieved successfully", dto)
}

func (h *DealHandler) GetMyDealRecommendations(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	props, err := h.service.GetMyDealRecommendations(c.Request.Context(), accountID, email, id, 10)
	if err != nil {
		respondPortalError(c, err)
		return
	}
	response.Success(c, "Recommendations retrieved successfully", props)
}

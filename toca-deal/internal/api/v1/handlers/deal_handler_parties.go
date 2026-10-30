package handlers

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// Party endpoints own identity, role, due-diligence and extraction mutations.

type UpdatePartyRequest struct {
	Role              *deal.PartyRole           `json:"role,omitempty"`
	NoSpouseDeclared  *bool                     `json:"no_spouse_declared,omitempty"`
	Representation    *deal.PartyRepresentation `json:"representation,omitempty"`
	SignatureRequired *bool                     `json:"signature_required,omitempty"`
	// Campos identificadores — editáveis pelo painel de partes da Esteira
	// para o operador completar o cadastro de uma parte. O documento
	// (CPF/CNPJ) em particular é exigido pelo gate de transição para
	// CONTRACT e não tinha caminho de edição antes.
	Name      *string `json:"name,omitempty"`
	Email     *string `json:"email,omitempty"`
	Phone     *string `json:"phone,omitempty"`
	Document  *string `json:"document,omitempty"`
	BirthDate *string `json:"birth_date,omitempty"`
	// CustomFields permite editar os campos livres da parte (ex.: profissão,
	// endereço) diretamente no painel. O serviço registra proveniência manual
	// apenas nos valores que realmente mudaram.
	CustomFields map[string]string `json:"custom_fields,omitempty"`
	// RemovedCustomFields são chaves que o operador removeu no painel. O
	// serviço apaga o valor e a proveniência correspondente.
	RemovedCustomFields []string `json:"removed_custom_fields,omitempty"`
}

func (h *DealHandler) ListDealParties(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	brokerID, _, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	parties, err := h.service.ListDealParties(c.Request.Context(), tenantID, id, brokerID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Deal parties retrieved successfully", parties)
}

func (h *DealHandler) AddParty(c *gin.Context) {
	id := c.Param("id")
	var req CreatePartyRequest
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
	d, err := h.service.AddParty(c.Request.Context(), tenantID, id, brokerID, deal.Party{
		LeadID:           req.LeadID,
		AccountID:        req.AccountID,
		Name:             req.Name,
		Email:            req.Email,
		Phone:            req.Phone,
		Document:         req.Document,
		Kind:             req.Kind,
		Role:             req.Role,
		SpouseOfPartyID:  req.SpouseOfPartyID,
		NoSpouseDeclared: req.NoSpouseDeclared,
		Representation:   req.Representation,
		SharePct:         req.SharePct,
		IsPrimary:        req.IsPrimary,
	}, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Party added successfully", d)
}

func (h *DealHandler) RemoveParty(c *gin.Context) {
	id := c.Param("id")
	partyID := c.Param("partyId")
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
	d, err := h.service.RemoveParty(c.Request.Context(), tenantID, id, brokerID, partyID, userID)
	if err != nil {
		if strings.HasPrefix(err.Error(), "cannot remove the only primary party for role") {
			role := ""
			parts := strings.Split(err.Error(), "'")
			if len(parts) >= 2 {
				role = parts[1]
			}
			roleLabel := role
			switch role {
			case string(deal.PartyRoleTenant):
				roleLabel = "inquilino"
			case string(deal.PartyRoleBuyer):
				roleLabel = "comprador"
			case string(deal.PartyRoleOwner):
				roleLabel = "proprietário"
			case string(deal.PartyRoleGuarantor):
				roleLabel = "fiador/avalista"
			case string(deal.PartyRoleWitness):
				roleLabel = "testemunha"
			}
			response.ValidationError(c, fmt.Sprintf("Não é possível remover a única parte primária do papel %s. Defina outra parte como primária antes de remover.", roleLabel))
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Party removed successfully", d)
}

func (h *DealHandler) UpdatePartyRole(c *gin.Context) {
	id := c.Param("id")
	partyID := c.Param("partyId")
	var req UpdatePartyRequest
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
	d, err := h.service.UpdateParty(c.Request.Context(), tenantID, id, brokerID, partyID, req.Role, req.NoSpouseDeclared, req.Representation, req.SignatureRequired, &service.PartyIdentityPatch{
		Name:                req.Name,
		Email:               req.Email,
		Phone:               req.Phone,
		Document:            req.Document,
		BirthDate:           req.BirthDate,
		CustomFields:        req.CustomFields,
		RemovedCustomFields: req.RemovedCustomFields,
	}, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Party updated successfully", d)
}

func (h *DealHandler) UpdatePartyDueDiligence(c *gin.Context) {
	id := c.Param("id")
	partyID := c.Param("partyId")
	var req UpdateDueDiligenceRequest
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

	d, err := h.service.UpdatePartyDueDiligence(c.Request.Context(), tenantID, id, brokerID, partyID, req.ScoreSerasa, req.CNDStatus, req.LegalOpinion, req.MonthlyIncome, req.Status, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Party due diligence updated successfully", d)
}

type UpdateDueDiligenceCriterionRequest struct {
	Status string `json:"status" binding:"required"`
	Notes  string `json:"notes"`
}

// UpdatePartyDueDiligenceCriterion registra a revisão de um critério
// (score, cnd, legal_opinion, income) e deriva o status da parte.
func (h *DealHandler) UpdatePartyDueDiligenceCriterion(c *gin.Context) {
	id := c.Param("id")
	partyID := c.Param("partyId")
	criterion := c.Param("criterion")
	var req UpdateDueDiligenceCriterionRequest
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
	d, err := h.service.UpdatePartyDueDiligenceCriterion(
		c.Request.Context(), tenantID, id, brokerID, partyID, criterion, req.Status, req.Notes, userID,
	)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Party due diligence criterion updated successfully", d)
}

// ApplyExtractionField é um campo identificado pela IA que o operador aceita
// (possivelmente corrigido) para gravar na party.
type ApplyExtractionField struct {
	Name       string  `json:"name"`
	Target     string  `json:"target,omitempty"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence,omitempty"`
}

// ApplyExtractionRequest é o corpo de POST /deals/:id/parties/:partyId/apply-extraction.
type ApplyExtractionRequest struct {
	DocumentID string                 `json:"document_id,omitempty"`
	Fields     []ApplyExtractionField `json:"fields"`
}

// ApplyExtraction grava na party os campos identificados pela IA que o operador
// validou/corrigiu, marcando a origem como "validated". É o caminho de correção
// de divergências e validação manual.
func (h *DealHandler) ApplyExtraction(c *gin.Context) {
	id := c.Param("id")
	partyID := c.Param("partyId")
	var req ApplyExtractionRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if len(req.Fields) == 0 {
		response.ValidationError(c, "fields é obrigatório")
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
	fields := make([]service.ExtractedFieldApply, 0, len(req.Fields))
	for _, f := range req.Fields {
		fields = append(fields, service.ExtractedFieldApply{
			Name:       f.Name,
			Target:     f.Target,
			Value:      f.Value,
			Confidence: f.Confidence,
		})
	}
	d, err := h.service.ApplyExtractedFields(c.Request.Context(), tenantID, id, brokerID, partyID, fields, deal.ProvenanceValidated, req.DocumentID, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Dados validados e aplicados à parte", d)
}

func (h *DealHandler) SetPrimaryParty(c *gin.Context) {
	id := c.Param("id")
	partyID := c.Param("partyId")
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
	d, err := h.service.SetPrimaryParty(c.Request.Context(), tenantID, id, brokerID, partyID, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Primary party updated successfully", d)
}

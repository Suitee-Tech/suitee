package handlers

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

type ContractHandler struct {
	service *service.ContractService
	docs    *service.DocumentService
}

func NewContractHandler(service *service.ContractService) *ContractHandler {
	return &ContractHandler{service: service}
}

// WithDocumentService wires the document side so handlers that need to
// resolve a stored MinIO copy (signed PDF view, etc.) can do it without a
// transitive contract-service dependency. Optional — routes that don't
// touch documents (Create/Update/Sign) keep working without it.
func (h *ContractHandler) WithDocumentService(docs *service.DocumentService) *ContractHandler {
	h.docs = docs
	return h
}

// Create godoc
// @Summary      Create contract
// @Description  Create a new contract for a deal
// @Tags         contracts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body contract.CreateContractRequest true "Create Contract Request"
// @Success      201  {object}  response.Response{data=contract.Contract}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /contracts [post]
func (h *ContractHandler) Create(c *gin.Context) {
	var req contract.CreateContractRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	contract, err := h.service.CreateContract(c.Request.Context(), tenantID, &req)
	if err != nil {
		respondContractCreateError(c, err)
		return
	}

	response.Created(c, "Contract created successfully", contract)
}

func respondContractCreateError(c *gin.Context, err error) {
	switch {
	case err.Error() == "unauthorized":
		response.Unauthorized(c, err.Error())
	case err.Error() == "deal not found":
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrSalePlanNotReady):
		// T-01: guard server-side do plano financeiro — mensagem operacional
		// direta (mesmo gate que a esteira aplica no frontend).
		response.UnprocessableEntity(c, err.Error())
	case contract.IsValidationError(err):
		response.ValidationError(c, err.Error())
	default:
		response.InternalError(c, err)
	}
}

// GetByID godoc
// @Summary      Get contract by ID
// @Description  Get a contract by its ID
// @Tags         contracts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Contract ID"
// @Success      200  {object}  response.Response{data=contract.Contract}
// @Failure      404  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /contracts/{id} [get]
func (h *ContractHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// P1/A25: leitura escopada pelo negócio do contrato — um corretor só lê o
	// contrato de negócio no escopo dele; admin/owner/manager sem recorte.
	contract, err := h.service.GetContract(c.Request.Context(), tenantID, id, callerBrokerScope(c))
	if err != nil {
		if err.Error() == "contract not found" {
			response.NotFound(c, err.Error())
		} else if err.Error() == "unauthorized" {
			response.Unauthorized(c, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contract retrieved successfully", contract)
}

func (h *ContractHandler) GetByIDInternal(c *gin.Context) {
	id := c.Param("id")
	// A3 (reauditoria de 2026-09-20): o tenant vinha cru do `?tenant_id=`, sem
	// amarra à credencial de serviço que autentica a rota — uma chave nomeada com
	// allowlist do tenant A lia o contrato do tenant B. Resolvedor único.
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

	// Rota interna: o chamador é serviço (ou serviço em nome de usuário) com o
	// tenant já resolvido pela credencial — não há escopo de corretor a aplicar.
	contract, err := h.service.GetContract(c.Request.Context(), tenantID, id, "")
	if err != nil {
		if err.Error() == "contract not found" {
			response.NotFound(c, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contract retrieved successfully", contract)
}

// ListByDeal godoc
// @Summary      List contracts by deal
// @Description  List all contracts associated with a deal
// @Tags         contracts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        deal_id path string true "Deal ID"
// @Success      200  {object}  response.Response{data=[]contract.Contract}
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /deals/{id}/contracts [get]
func (h *ContractHandler) ListByDeal(c *gin.Context) {
	dealID := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// P1/A25: contratos do negócio são lidos no escopo do chamador.
	//
	// Negócio fora do escopo do corretor é "não encontrado" para ele, não erro
	// interno: a comparação era por TEXTO ("deal not found or unauthorized") e o
	// serviço devolve `ErrDealNotInScope` ("deal not in caller scope"), então o
	// caso caía no braço de 500 e poluía o log de produção.
	contracts, err := h.service.ListContractsByDeal(c.Request.Context(), tenantID, dealID, callerBrokerScope(c))
	if err != nil {
		if errors.Is(err, service.ErrDealNotInScope) {
			response.NotFound(c, "deal not found")
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contracts retrieved successfully", contracts)
}

// Update godoc
// @Summary      Update contract
// @Tags         contracts
// @Security     BearerAuth
// @Param        id path string true "Contract ID"
// @Success      200  {object}  response.Response{data=contract.Contract}
// @Router       /contracts/{id} [put]
func (h *ContractHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req contract.CreateContractRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	updated, err := h.service.UpdateContract(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		switch {
		case err.Error() == "contract not found":
			response.NotFound(c, err.Error())
		case err.Error() == "unauthorized":
			response.Unauthorized(c, err.Error())
		case errors.Is(err, service.ErrContractLockedForSignature), errors.Is(err, contract.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contract updated successfully", updated)
}

// Regenerate replaces the unsigned draft while retaining its previous version.
func (h *ContractHandler) Regenerate(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req service.RegenerateContractRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	updated, err := h.service.RegenerateContract(c.Request.Context(), tenantID, c.Param("id"), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContractCannotRegenerate), errors.Is(err, contract.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		case errors.Is(err, service.ErrInvalidRegeneration), contract.IsValidationError(err):
			response.ValidationError(c, err.Error())
		case err.Error() == "contract not found":
			response.NotFound(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}
	response.Success(c, "Contrato regerado. Revise e envie para assinatura.", updated)
}

// Delete godoc
// @Summary      Delete contract
// @Tags         contracts
// @Security     BearerAuth
// @Param        id path string true "Contract ID"
// @Success      200  {object}  response.Response
// @Router       /contracts/{id} [delete]
func (h *ContractHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	if err := h.service.DeleteContract(c.Request.Context(), tenantID, id); err != nil {
		if err.Error() == "contract not found" {
			response.NotFound(c, err.Error())
		} else if err.Error() == "unauthorized" {
			response.Unauthorized(c, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contract deleted successfully", nil)
}

// List
func (h *ContractHandler) List(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	contracts, err := h.service.ListContracts(c.Request.Context(), tenantID, nil)
	if err != nil {
		if err.Error() == "unauthorized" {
			response.Unauthorized(c, err.Error())
		} else if isNotImplemented(err) {
			response.Success(c, "Contracts retrieved successfully", []*contract.Contract{})
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contracts retrieved successfully", contracts)
}

type SignContractRequest struct {
	SignatoryID string `json:"signatory_id" binding:"required"`
	Signature   string `json:"signature" binding:"required"`
}

// SendForSignatureRequest is the HTTP shape for POST /contracts/:id/send-for-signature.
// SignerAuth maps Contract.Signatory.ID → list of auth methods (email, whatsapp,
// sms, selfie, icp_brasil, etc.). Empty entries fall back to ["email"].
//
// SignerGroups + AttachmentDocIDs + InlineAttachments add multi-doc and signing
// order support: see the matching fields on toca-signature's
// CreateEnvelopeForContractRequest for semantics. All three are optional and
// default to legacy single-doc/parallel behavior when omitted.
type SendForSignatureRequest struct {
	SignerAuth        map[string][]string         `json:"signer_auth"`
	SignerGroups      map[string]int              `json:"signer_groups,omitempty"`
	AttachmentDocIDs  []string                    `json:"attachment_document_ids,omitempty"`
	InlineAttachments []SendInlineAttachmentInput `json:"inline_attachments,omitempty"`
	Locale            string                      `json:"locale,omitempty"`
}

// SendInlineAttachmentInput is the wire shape for an operator-uploaded PDF.
// ContentBase64 is the standard base64 string (no data: prefix required —
// the signature service strips it either way).
type SendInlineAttachmentInput struct {
	Filename      string `json:"filename"`
	ContentBase64 string `json:"content_base64"`
}

// SendForSignature godoc
// @Summary      Send contract for external signature (Clicksign)
// @Description  Locks the contract for edits, transitions to PENDING_SIGNATURE,
// @Description  and publishes signature.requested for the signature service to process.
// @Tags         contracts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                  true "Contract ID"
// @Param        request body  SendForSignatureRequest true "Auth methods per signer"
// @Success      200  {object}  response.Response{data=contract.Contract}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      409  {object}  response.Response  "contract is locked for signature"
// @Router       /contracts/{id}/send-for-signature [post]
func (h *ContractHandler) SendForSignature(c *gin.Context) {
	id := c.Param("id")
	var req SendForSignatureRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	inline := make([]service.SendInlineAttachment, 0, len(req.InlineAttachments))
	for _, in := range req.InlineAttachments {
		inline = append(inline, service.SendInlineAttachment{
			Filename:      in.Filename,
			ContentBase64: in.ContentBase64,
		})
	}
	updated, err := h.service.SendForSignature(c.Request.Context(), tenantID, id, service.SendForSignatureRequest{
		SignerAuth:        req.SignerAuth,
		SignerGroups:      req.SignerGroups,
		AttachmentDocIDs:  req.AttachmentDocIDs,
		InlineAttachments: inline,
		Locale:            req.Locale,
	})
	if err != nil {
		switch {
		case err.Error() == "contract not found":
			response.NotFound(c, err.Error())
		case errors.Is(err, service.ErrSalePlanNotReady):
			// T-01: venda sem plano de recebimento ativo/compatível não sai
			// para assinatura (o contract.signed não fabrica fallback).
			response.UnprocessableEntity(c, err.Error())
		case errors.Is(err, service.ErrContractLockedForSignature), errors.Is(err, contract.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}
	response.Success(c, "Contract sent for signature", updated)
}

// Sign godoc
// @Summary      Sign contract
// @Description  Sign a contract as a signatory
// @Tags         contracts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Contract ID"
// @Param        request body SignContractRequest true "Sign Contract Request"
// @Success      200  {object}  response.Response{data=contract.Contract}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /contracts/{id}/sign [post]
func (h *ContractHandler) Sign(c *gin.Context) {
	id := c.Param("id")
	var req SignContractRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// SECURITY (C12): a identidade do assinante vem do JWT verificado, nunca do
	// corpo. O serviço confronta com o signatário-alvo antes de assinar.
	callerAccountID, _ := context.GetAccountID(c.Request.Context())
	callerEmail, _ := context.GetEmail(c.Request.Context())
	signer := service.SignerIdentity{AccountID: callerAccountID, Email: callerEmail}

	contract, err := h.service.SignContract(c.Request.Context(), tenantID, id, req.SignatoryID, req.Signature, signer)
	if err != nil {
		if err.Error() == "contract not found" {
			response.NotFound(c, err.Error())
		} else if err.Error() == "unauthorized" {
			response.Forbidden(c, "signatory identity does not match the authenticated caller")
		} else if err.Error() == "signatory not found in contract" {
			response.ValidationError(c, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contract signed successfully", contract)
}

// SignedFileResponse is the shape returned by GET /contracts/:id/signed-file.
// `url` is the most durable URL the system can offer at the moment — a MinIO
// presigned URL when the PersistSignedContractPDF subscriber has already
// cached the file locally, or the Clicksign-hosted URL as a fallback during
// the brief window after auto_close but before persistence completes.
// `source` lets the caller distinguish (and future-cache invalidation).
type SignedFileResponse struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	Status string `json:"status,omitempty"`
}

// GetSignedFile godoc
// @Summary      Get the signed contract PDF URL
// @Description  Returns the URL of the final signed PDF (post-Clicksign).
// @Description  Prefers the persisted MinIO presigned URL (durable, internal)
// @Description  and falls back to the Clicksign-hosted URL while persistence
// @Description  is still in flight. Returns 404 if the contract isn't SIGNED
// @Description  yet or if no URL is available.
// @Tags         contracts
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Contract ID"
// @Success      200 {object} response.Response{data=SignedFileResponse}
// @Failure      404 {object} response.Response
// @Router       /contracts/{id}/signed-file [get]
func (h *ContractHandler) GetSignedFile(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// P1/A25: mesma leitura escopada da rota de detalhe.
	contractObj, err := h.service.GetContract(c.Request.Context(), tenantID, id, callerBrokerScope(c))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if contractObj == nil {
		response.NotFound(c, "contract not found")
		return
	}

	// Always serve the signed PDF from our own storage (MinIO). The raw
	// Clicksign S3 URL is unusable in the browser — its bucket sends no CORS
	// header for the app origin and the presigned link expires in ~5 min, so
	// the PDF viewer fails with a CORS/403 error. EnsureSignedPDFURL
	// downloads and caches the PDF server-side on first access when needed.
	if h.docs != nil {
		if url, err := h.docs.EnsureSignedPDFURL(c.Request.Context(), tenantID, id); err == nil && url != "" {
			response.Success(c, "Signed file URL", SignedFileResponse{
				URL:    url,
				Source: "minio",
				Status: string(contractObj.Status),
			})
			return
		}
	}
	response.NotFound(c, "signed file not available yet")
}

// TerminateContractRequest is the wire shape for POST /contracts/:id/terminate.
// All fields are optional except Reason (which is enforced at the handler
// level so the audit trail always carries an operator-provided justification).
type TerminateContractRequest struct {
	Reason                    string  `json:"reason" binding:"required"`
	EarlyTerminationFee       float64 `json:"early_termination_fee"`
	SecurityDepositReturn     float64 `json:"security_deposit_return"`
	SecurityDepositDeductions float64 `json:"security_deposit_deductions"`
	ExitInspectionID          string  `json:"exit_inspection_id"`
	RequireExitInspection     bool    `json:"require_exit_inspection"`
}

// Terminate godoc
// @Summary      Terminate (encerrar) rental contract
// @Description  Marks the contract as TERMINATED, records reason/fee/deposit
// @Description  acerto, and publishes contract.terminated for toca-financial
// @Description  to cancel future recurring charges and process the caução.
// @Tags         contracts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                    true "Contract ID"
// @Param        request body  TerminateContractRequest  true "Termination request"
// @Success      200  {object}  response.Response{data=contract.Contract}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      409  {object}  response.Response  "exit inspection required"
// @Router       /contracts/{id}/terminate [post]
func (h *ContractHandler) Terminate(c *gin.Context) {
	id := c.Param("id")
	var req TerminateContractRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	updated, err := h.service.TerminateContract(c.Request.Context(), tenantID, id, service.TerminateContractRequest{
		Reason:                    req.Reason,
		EarlyTerminationFee:       req.EarlyTerminationFee,
		SecurityDepositReturn:     req.SecurityDepositReturn,
		SecurityDepositDeductions: req.SecurityDepositDeductions,
		ExitInspectionID:          req.ExitInspectionID,
		RequireExitInspection:     req.RequireExitInspection,
	})
	if err != nil {
		switch {
		case err.Error() == "contract not found":
			response.NotFound(c, err.Error())
		case errors.Is(err, service.ErrExitInspectionRequired):
			response.Conflict(c, err.Error())
		case err.Error() == "contract already terminated" || err.Error() == "contract cannot be terminated before signature":
			response.ValidationError(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contract terminated", updated)
}

// PreviewTerminationFee godoc
// @Summary      Calcular multa de rescisão antecipada (#438)
// @Description  Aplica a fórmula canônica da Lei 8245/91 Art. 4 — multa
// @Description  proporcional decrescente: 3 × aluguel × (meses_restantes /
// @Description  total). Endpoint pensado para o portal cliente preview ANTES
// @Description  de solicitar encerramento, e para o admin reconciliar o
// @Description  número que vai aparecer no Terminate. Não muta nada.
// @Tags         contracts
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string true  "Contract ID"
// @Param        date query string false "Data hipotética de encerramento (YYYY-MM-DD). Default = hoje."
// @Success      200  {object}  response.Response{data=contract.TerminationFeeBreakdown}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /contracts/{id}/termination-fee-preview [get]
func (h *ContractHandler) PreviewTerminationFee(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// Carrega o contrato pra extrair start/end/value. Reusa GetContract
	// (já tem o gate de "contract not found" + permissão por tenant).
	// P1/A25: a prévia lê o mesmo contrato — mesma escopagem da rota de detalhe.
	ct, err := h.service.GetContract(c.Request.Context(), tenantID, id, callerBrokerScope(c))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if ct == nil {
		response.NotFound(c, "contract not found")
		return
	}

	// Data hipotética. Default = hoje, hora UTC zerada para que multas
	// calculadas com a mesma data calendário não variem por timezone do
	// cliente.
	terminate := time.Now().UTC()
	if raw := c.Query("date"); raw != "" {
		parsed, perr := time.Parse("2006-01-02", raw)
		if perr != nil {
			response.ValidationError(c, "date deve ser YYYY-MM-DD: "+perr.Error())
			return
		}
		terminate = parsed
	}

	// EndDate é ponteiro nullable. Se ausente, a fórmula trata como
	// open-ended e devolve teto + reason="no_end_date". Não vira erro
	// HTTP — UI mostra warning.
	endDate := ct.StartDate
	if ct.EndDate != nil {
		endDate = *ct.EndDate
	}

	breakdown, err := contract.CalculateEarlyTerminationFee(
		contract.FloatToCents(ct.Value),
		ct.StartDate,
		endDate,
		terminate,
	)
	if err != nil {
		// ErrInvalidTerminationDate é o único caso. Trata como 400.
		response.ValidationError(c, err.Error())
		return
	}

	response.Success(c, "Termination fee calculated", breakdown)
}

// AmendContractRequest is the wire shape for POST /contracts/:id/amend. All
// fields except `reason` are optional — when omitted, the amendment inherits
// from the original contract. The amendment is returned in DRAFT status;
// the operator runs the signature flow on the new contract as usual.
type AmendContractRequest struct {
	Reason             string               `json:"reason" binding:"required"`
	StartDate          *time.Time           `json:"start_date,omitempty"`
	EndDate            *time.Time           `json:"end_date,omitempty"`
	Value              float64              `json:"value,omitempty"`
	LateFeePct         float64              `json:"late_fee_pct,omitempty"`
	MonthlyInterestPct float64              `json:"monthly_interest_pct,omitempty"`
	AdjustIndex        string               `json:"adjust_index,omitempty"`
	AutoAdjust         *bool                `json:"auto_adjust,omitempty"`
	NextAdjustment     string               `json:"next_adjustment,omitempty"`
	SecurityDeposit    float64              `json:"security_deposit,omitempty"`
	Content            string               `json:"content,omitempty"`
	Signatories        []contract.Signatory `json:"signatories,omitempty"`
}

// Amend godoc
// @Summary      Issue contract amendment (aditivo)
// @Description  Creates a new DRAFT contract that inherits unchanged fields
// @Description  from the original. The amendment links back via
// @Description  amends_contract_id. The original keeps ACTIVE until the
// @Description  amendment is signed; envelope_finished on the amendment
// @Description  expires the original.
// @Tags         contracts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                true "Original contract ID"
// @Param        request body  AmendContractRequest  true "Amendment request"
// @Success      201  {object}  response.Response{data=contract.Contract}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /contracts/{id}/amend [post]
func (h *ContractHandler) Amend(c *gin.Context) {
	id := c.Param("id")
	var req AmendContractRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	amendment, err := h.service.AmendContract(c.Request.Context(), tenantID, id, service.AmendContractRequest{
		Reason:             req.Reason,
		StartDate:          req.StartDate,
		EndDate:            req.EndDate,
		Value:              req.Value,
		LateFeePct:         req.LateFeePct,
		MonthlyInterestPct: req.MonthlyInterestPct,
		AdjustIndex:        req.AdjustIndex,
		AutoAdjust:         req.AutoAdjust,
		NextAdjustment:     req.NextAdjustment,
		SecurityDeposit:    req.SecurityDeposit,
		Content:            req.Content,
		Signatories:        req.Signatories,
	})
	if err != nil {
		switch err.Error() {
		case "contract not found":
			response.NotFound(c, err.Error())
		case "amendment reason is required",
			"can only amend an active (SIGNED/ACTIVE) contract",
			"cannot amend an amendment — start from the original contract":
			response.ValidationError(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Created(c, "Amendment created", amendment)
}

// ReadjustContractRequest is the wire shape for POST /contracts/:id/readjust.
// EffectiveFrom defaults to now (UTC) when zero. NewValue takes precedence
// over IndexValue when both are sent.
type ReadjustContractRequest struct {
	NewValue       float64    `json:"new_value"`
	IndexName      string     `json:"index_name"`
	IndexValue     float64    `json:"index_value"`
	EffectiveFrom  *time.Time `json:"effective_from"`
	NextAdjustment string     `json:"next_adjustment"`
	Note           string     `json:"note"`
}

// Readjust godoc
// @Summary      Readjust (reajustar) rental contract value
// @Description  Applies a manual readjustment to the rent value and publishes
// @Description  contract.readjusted so toca-financial updates the recurring
// @Description  monthly amount in RentalChargeConfig. Existing charges are
// @Description  NOT modified — only future generations pick up the new value.
// @Tags         contracts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                   true "Contract ID"
// @Param        request body  ReadjustContractRequest  true "Readjustment request"
// @Success      200  {object}  response.Response{data=contract.Contract}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /contracts/{id}/readjust [post]
func (h *ContractHandler) Readjust(c *gin.Context) {
	id := c.Param("id")
	var req ReadjustContractRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	svcReq := service.ReadjustContractRequest{
		NewValue:       req.NewValue,
		IndexName:      req.IndexName,
		IndexValue:     req.IndexValue,
		NextAdjustment: req.NextAdjustment,
		Mode:           "manual",
		Note:           req.Note,
	}
	if req.EffectiveFrom != nil {
		svcReq.EffectiveFrom = *req.EffectiveFrom
	}

	updated, err := h.service.ReadjustContract(c.Request.Context(), tenantID, id, svcReq)
	if err != nil {
		switch err.Error() {
		case "contract not found":
			response.NotFound(c, err.Error())
		case "readjust requires new_value or index_value",
			"new_value must be positive",
			"contract is not active; cannot apply adjustment":
			response.ValidationError(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Contract readjusted", updated)
}

package handlers

import (
	stdctx "context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// Force usage of transfer package for Swagger
var _ = transfer.OwnerTransfer{}

type TransferHandler struct {
	service     *service.TransferService
	dealClient  *clients.DealClient
	crmClient   *clients.CrmClient
	proofPolicy proofPolicySettings
	// tenantCompany resolve o nome da imobiliária no demonstrativo de repasse.
	// Opcional: sem ele o PDF mantém o fallback neutro.
	tenantCompany tenantCompanyLookup
}

// SetProofPolicy conecta a política de comprovante do tenant para a efetivação
// manual do repasse. Sem isso vale o default do domínio, que exige.
func (h *TransferHandler) SetProofPolicy(settings proofPolicySettings) {
	h.proofPolicy = settings
}

// WithTenantCompanyLookup injeta o resolvedor do nome da imobiliária logada.
func (h *TransferHandler) WithTenantCompanyLookup(lookup tenantCompanyLookup) *TransferHandler {
	h.tenantCompany = lookup
	return h
}

func NewTransferHandler(svc *service.TransferService) *TransferHandler {
	return &TransferHandler{service: svc}
}

func getTransferIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := c.GetHeader("Idempotency-Key"); v != "" {
		return v
	}
	return c.GetHeader("X-Idempotency-Key")
}

// SetDealClient wires an optional deal client so the receipt PDF can include
// real owner/tenant names and the contract's denormalised data. Nil-safe:
// without it the PDF still renders, just with raw IDs.
func (h *TransferHandler) SetDealClient(dc *clients.DealClient) {
	h.dealClient = dc
}

// SetCrmClient wires an optional CRM client used by the receipt PDF to
// resolve the property owner's name (and other contact details). Nil-safe.
func (h *TransferHandler) SetCrmClient(cc *clients.CrmClient) {
	h.crmClient = cc
}

type CreateTransferRequest struct {
	ContractID            string               `json:"contract_id" binding:"required"`
	OwnerID               string               `json:"owner_id" binding:"required"`
	PropertyID            string               `json:"property_id" binding:"required"`
	ReferenceMonth        string               `json:"reference_month" binding:"required"`
	GrossRent             float64              `json:"gross_rent" binding:"required"`
	AdminFeePercent       float64              `json:"admin_fee_percent" binding:"required"`
	MaintenanceDeductions float64              `json:"maintenance_deductions"`
	OtherDeductions       float64              `json:"other_deductions"`
	DeductionDetails      []transfer.Deduction `json:"deduction_details"`
	Notes                 string               `json:"notes"`
}

// Create godoc
// @Summary      Create owner transfer
// @Description  Create a new owner transfer
// @Tags         transfers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateTransferRequest true "Create Transfer Request"
// @Success      201  {object}  response.Response{data=transfer.OwnerTransfer}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /transfers [post]
func (h *TransferHandler) Create(c *gin.Context) {
	var req CreateTransferRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	idempotencyKey := getTransferIdempotencyKey(c)
	var t *transfer.OwnerTransfer
	if idempotencyKey != "" {
		t, err = h.service.CreateIdempotent(
			c.Request.Context(),
			req.ContractID,
			req.OwnerID,
			req.PropertyID,
			tenantID,
			req.ReferenceMonth,
			req.GrossRent,
			req.AdminFeePercent,
			req.MaintenanceDeductions,
			req.OtherDeductions,
			req.DeductionDetails,
			req.Notes,
		)
	} else {
		t, err = h.service.Create(
			c.Request.Context(),
			req.ContractID,
			req.OwnerID,
			req.PropertyID,
			tenantID,
			req.ReferenceMonth,
			req.GrossRent,
			req.AdminFeePercent,
			req.MaintenanceDeductions,
			req.OtherDeductions,
			req.DeductionDetails,
			req.Notes,
		)
	}
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Created(c, "Transfer created successfully", t)
}

// List godoc
// @Summary      List transfers
// @Description  List owner transfers with pagination and filters
// @Tags         transfers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        contract_id query string false "Contract ID"
// @Param        owner_id query string false "Owner ID"
// @Param        status query string false "Status"
// @Param        reference_month query string false "Reference Month"
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response{data=[]transfer.OwnerTransfer}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /transfers [get]
func (h *TransferHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	contractID := c.Query("contract_id")
	ownerID := c.Query("owner_id")
	status := c.Query("status")
	referenceMonth := c.Query("reference_month")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
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
	dateFrom, err := parseTime(c.Query("date_from"))
	if err != nil {
		response.ValidationError(c, "invalid date_from value")
		return
	}
	dateTo, err := parseTime(c.Query("date_to"))
	if err != nil {
		response.ValidationError(c, "invalid date_to value")
		return
	}

	filter := transfer.SearchFilter{
		ContractID:     contractID,
		OwnerID:        ownerID,
		BrokerID:       c.Query("broker_id"),
		Status:         status,
		ReferenceMonth: referenceMonth,
		MinAmount:      minAmount,
		MaxAmount:      maxAmount,
		DateFrom:       dateFrom,
		DateTo:         dateTo,
		Limit:          limit,
		Offset:         offset,
	}

	transfers, total, err := h.service.Search(c.Request.Context(), tenantID, filter)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Transfers retrieved successfully", transfers, page, int(limit), total)
}

// GetByID godoc
//
// SECURITY: este endpoint é gated por permission `finance:read` +
// RejectFunctionalRole("cliente"), ou seja, apenas admin/gestor/operações/
// financeiro chegam aqui. Esses perfis legitimamente precisam ver
// repasses de todos os owners do tenant — NÃO adicionar filtro por
// account_id aqui. Portal owner usa
// PortalTransferHandler.GetMine que valida ownership por account_id.
//
// @Summary      Get transfer by ID
// @Description  Get an owner transfer by its ID
// @Tags         transfers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Transfer ID"
// @Success      200  {object}  response.Response{data=transfer.OwnerTransfer}
// @Failure      404  {object}  response.Response
// @Router       /transfers/{id} [get]
func (h *TransferHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	t, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Transfer retrieved successfully", t)
}

// Approve godoc
// @Summary      Approve transfer
// @Description  Approve an owner transfer
// @Tags         transfers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Transfer ID"
// @Success      200  {object}  response.Response{data=transfer.OwnerTransfer}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      409  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /transfers/{id}/approve [post]
func (h *TransferHandler) Approve(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	// Capture the operator id + display name for the audit trail. Missing
	// account_id (e.g. internal-key callers) falls back to "system" so the
	// history stays interpretable. user_name comes from the JWT name claim
	// when present.
	actor, _ := context.GetAccountID(c.Request.Context())
	if actor == "" {
		actor = "system"
	}
	actorLabel, _ := context.GetUserName(c.Request.Context())
	// P1/A13: quem responde pela operação financeira (admin/gestor) segue
	// aprovando sozinho; a dispensa vem do papel autenticado, nunca do corpo.
	ctx := withFinanceAuthorityWaiver(c.Request.Context())
	if err := h.service.Approve(ctx, tenantID, id, actor, actorLabel); err != nil {
		if err.Error() == "transfer not found" {
			response.NotFound(c, err.Error())
		} else if errors.Is(err, transfer.ErrConcurrentModification) || isSegregationOfDutiesError(err) ||
			errors.Is(err, transfer.ErrTransferHeldByDelinquency) {
			// Retenção por inadimplência é conflito de estado (não erro do
			// chamador): 409 com o motivo, para o operador saber o que resolver.
			response.Conflict(c, err.Error())
		} else if err.Error() == "transfer must be in PENDING status to approve" {
			response.ValidationError(c, err.Error())
		} else {
			respondInternalError(c, err)
		}
		return
	}
	updated, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Transfer approved successfully", updated)
}

// Process godoc
// @Summary      Process transfer
// @Description  Process an approved owner transfer
// @Tags         transfers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Transfer ID"
// @Success      200  {object}  response.Response{data=transfer.OwnerTransfer}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      409  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /transfers/{id}/process [post]
// transferHasNoPayableBalance informa se o repasse é apenas um encerramento de
// obrigação zerada. Erros de leitura são tratados como "tem saldo": nesse caso
// a política de comprovante continua valendo e o serviço responde o erro real.
func (h *TransferHandler) transferHasNoPayableBalance(ctx stdctx.Context, tenantID, id string) bool {
	if h.service == nil || strings.TrimSpace(id) == "" {
		return false
	}
	item, err := h.service.GetByID(ctx, tenantID, id)
	if err != nil || item == nil {
		return false
	}
	return item.HasNoPayableBalance()
}

func (h *TransferHandler) Process(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	actor, _ := context.GetAccountID(c.Request.Context())
	if actor == "" {
		actor = "system"
	}
	actorLabel, _ := context.GetUserName(c.Request.Context())
	// Corpo opcional: quando o repasse é efetivado manualmente (fluxo externo/
	// banco), o operador pode anexar o comprovante e/ou uma observação.
	var req struct {
		ProofURL string `json:"proof_url"`
		Note     string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)
	// Efetivar o repasse registra um depósito feito fora do sistema. Sem
	// evidência o registro não distingue "paguei" de "pretendo pagar".
	//
	// Repasse sem saldo a repassar é a exceção: não há pagamento a comprovar, a
	// baixa apenas encerra a obrigação zerada. Exigir comprovante aqui deixaria
	// o registro travado para sempre em tenants que exigem evidência na baixa
	// manual — exatamente o caso dos repasses consumidos por contas
	// administradas/deduções.
	if !h.transferHasNoPayableBalance(c.Request.Context(), tenantID, id) {
		if err := requireManualProof(c.Request.Context(), h.proofPolicy, tenantID, req.ProofURL); err != nil {
			response.ValidationError(c, err.Error())
			return
		}
	}
	// P1/A13: pagador ≠ aprovador (com a mesma dispensa de autoridade financeira).
	if err := h.service.ProcessWithEvidence(withFinanceAuthorityWaiver(c.Request.Context()), tenantID, id, actor, actorLabel, req.ProofURL, req.Note); err != nil {
		if err.Error() == "transfer not found" {
			response.NotFound(c, err.Error())
		} else if errors.Is(err, transfer.ErrConcurrentModification) ||
			isSegregationOfDutiesError(err) ||
			errors.Is(err, service.ErrTransferSettlementJournalMissing) ||
			errors.Is(err, service.ErrTransferSettlementCoverageRequired) ||
			errors.Is(err, service.ErrTransferSettlementAmountMismatch) ||
			errors.Is(err, service.ErrTransferWithoutPayableBalanceNotOpen) ||
			errors.Is(err, transfer.ErrTransferHeldByDelinquency) ||
			errors.Is(err, journal.ErrSourceEventConflict) {
			response.Conflict(c, err.Error())
		} else if err.Error() == "transfer must be in APPROVED status to process" {
			response.ValidationError(c, err.Error())
		} else if errors.Is(err, service.ErrTransferSettlementAmountInvalid) {
			response.ValidationError(c, err.Error())
		} else {
			respondInternalError(c, err)
		}
		return
	}
	updated, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Transfer processed successfully", updated)
}

// Receipt godoc
// @Summary      Download transfer receipt PDF
// @Description  Returns the "Demonstrativo de Repasse" as a PDF for download.
// @Tags         transfers
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        id path string true "Transfer ID"
// @Success      200  {file}  binary
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /transfers/{id}/receipt [get]
func (h *TransferHandler) Receipt(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	t, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil || t == nil {
		response.NotFound(c, "transfer not found")
		return
	}

	rctx := service.TransferReceiptContext{Transfer: t}
	// Best-effort enrichment. We swallow lookup errors so the receipt still
	// renders for legacy contracts/leads that the upstream services can't
	// resolve — the PDF degrades to raw IDs, never to a 500.
	if h.dealClient != nil && t.ContractID != "" {
		if cdto, derr := h.dealClient.GetContract(c.Request.Context(), tenantID, t.ContractID); derr == nil && cdto != nil {
			rctx.TenantName = cdto.Client
			rctx.PropertyTitle = cdto.PropertyTitle
		}
	}
	if h.crmClient != nil && t.OwnerID != "" {
		if owner, oerr := h.crmClient.GetLead(c.Request.Context(), tenantID, t.OwnerID); oerr == nil && owner != nil {
			rctx.OwnerName = owner.Name
			rctx.OwnerDocument = owner.Documentation
		}
	}
	if h.tenantCompany != nil {
		if name, lookupErr := h.tenantCompany.CompanyName(c.Request.Context(), tenantID); lookupErr == nil {
			rctx.OrgName = name
		}
	}

	pdf, err := service.RenderTransferReceiptPDF(rctx)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	filename := fmt.Sprintf("repasse_%s_%s.pdf", strings.ReplaceAll(t.ReferenceMonth, "/", "-"), shortID(t.ID))
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", pdf)
}

func shortID(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:8]
}

// ReleaseDelinquencyHoldRequest — corpo de POST /transfers/:id/release-delinquency-hold.
type ReleaseDelinquencyHoldRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// ReleaseDelinquencyHold libera a retenção por inadimplência de um repasse.
//
// Dois caminhos liberam: o automático (o caso de cobrança do contrato foi
// resolvido — a razão da retenção desapareceu) e este, manual, para quando quem
// tem alçada de repasse decide assumir o risco. Liberar NÃO paga: o pagamento
// continua exigindo a aprovação normal.
func (h *TransferHandler) ReleaseDelinquencyHold(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req ReleaseDelinquencyHoldRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	actor, _ := context.GetAccountID(ctx)
	t, err := h.service.ReleaseDelinquencyHold(ctx, tenantID, c.Param("id"), strings.TrimSpace(req.Reason), actor)
	if err != nil {
		if err.Error() == "transfer not found" {
			response.NotFound(c, err.Error())
			return
		}
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Retenção liberada; o repasse volta ao fluxo normal de aprovação", t)
}

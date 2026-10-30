package handlers

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// Force usage of collection package for Swagger
var _ = collection.Collection{}

type CollectionHandler struct {
	service *service.CollectionService
	// permProvider habilita a segunda assinatura do acordo (mesmo gate M11 das
	// cobranças: desconto acima do limiar exige `finance:approve_adjustment`).
	permProvider auth.PermissionProvider
}

func NewCollectionHandler(service *service.CollectionService) *CollectionHandler {
	return &CollectionHandler{service: service}
}

// WithPermissionProvider injeta o provedor usado para checar alçada de desconto.
func (h *CollectionHandler) WithPermissionProvider(p auth.PermissionProvider) *CollectionHandler {
	h.permProvider = p
	return h
}

func getCollectionIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := strings.TrimSpace(c.GetHeader("Idempotency-Key")); v != "" {
		return v
	}
	return strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
}

type CreateCollectionRequest struct {
	ContractID  string   `json:"contract_id" binding:"required"`
	DebtorID    string   `json:"debtor_id" binding:"required"`
	DebtorName  string   `json:"debtor_name" binding:"required"`
	TotalDebt   float64  `json:"total_debt" binding:"required,gt=0"`
	DaysOverdue int      `json:"days_overdue" binding:"required,gt=0"`
	ChargeIDs   []string `json:"charge_ids" binding:"required"`
}

type UpdateCollectionRequest struct {
	DebtorName       string            `json:"debtor_name"`
	TotalDebt        float64           `json:"total_debt"`
	DaysOverdue      int               `json:"days_overdue"`
	Status           collection.Status `json:"status"`
	NegotiatedAmount *float64          `json:"negotiated_amount,omitempty"`
	NegotiatedDate   *time.Time        `json:"negotiated_date,omitempty"`
	Notes            string            `json:"notes"`
}

// AddActionRequest — corpo de POST /collections/:id/actions.
//
// `user_id` deixou de ser obrigatório: era exigido do cliente e a tela de
// inadimplência não o enviava, então registrar um procedimento de cobrança
// devolvia 400 em silêncio. O autor agora vem do token (o cliente não escolhe
// quem assinou a ação); o campo continua aceito para importação de histórico.
type AddActionRequest struct {
	Type        string `json:"type" binding:"required"`
	Description string `json:"description" binding:"required"`
	UserID      string `json:"user_id"`
}

// JudicialProcessRequest — corpo de POST /collections/:id/judicial (#448).
// O mesmo endpoint cria e atualiza: reenviar atualiza os campos.
type JudicialProcessRequest struct {
	CaseNumber  string     `json:"case_number" binding:"required"` // número CNJ
	Court       string     `json:"court" binding:"required"`       // vara / comarca
	LawyerName  string     `json:"lawyer_name"`
	LawyerOAB   string     `json:"lawyer_oab"`
	LawyerEmail string     `json:"lawyer_email"`
	LawyerPhone string     `json:"lawyer_phone"`
	ClaimValue  float64    `json:"claim_value"`
	Status      string     `json:"status"` // vazio → EM_TRAMITE
	NextHearing *time.Time `json:"next_hearing,omitempty"`
	Notes       string     `json:"notes"`
}

// ActivateGuaranteeRequest — corpo de POST /collections/:id/guarantee (#447).
type ActivateGuaranteeRequest struct {
	Type   string  `json:"type" binding:"required"`        // SEGURO_FIANCA | FIADOR | CAUCAO
	Amount float64 `json:"amount" binding:"required,gt=0"` // valor acionado
	Reason string  `json:"reason"`
}

// UpdateGuaranteeStatusRequest — corpo de PUT /collections/:id/guarantee/status
// (#447). Usado pelo operador (ou por um webhook de seguradora) para
// registrar a resposta do garantidor sobre um acionamento já aberto.
type UpdateGuaranteeStatusRequest struct {
	Status string `json:"status" binding:"required"` // REQUESTED|ACCEPTED|REJECTED|PAID|CANCELLED
	// SettledAmount é o valor efetivamente recebido do garantidor. Quando o
	// status é PAID e este campo vem preenchido, o sistema dá baixa nas
	// cobranças do caso (mais antiga primeiro) — antes, marcar PAID era só
	// gravar um status e a dívida continuava vencida (G1 do plano).
	SettledAmount float64 `json:"settled_amount,omitempty"`
	// SettlementReference identifica o recebimento (protocolo do sinistro,
	// número do depósito judicial). Entra na chave de idempotência da baixa.
	SettlementReference string `json:"settlement_reference,omitempty"`
	InsurerResponse     string `json:"insurer_response"`
}

// Create godoc
// @Summary      Create collection
// @Description  Create a new collection case
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateCollectionRequest true "Create Collection Request"
// @Success      201  {object}  response.Response{data=collection.Collection}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /collections [post]
func (h *CollectionHandler) Create(c *gin.Context) {
	var req CreateCollectionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// FIN-COM-12: idempotência via header Idempotency-Key.
	// Deriva source_event_id determinístico incluindo contract_id,
	// debtor_id, e total_debt para que re-submissões com o mesmo header
	// mas valores diferentes sejam tratadas como operações distintas.
	idempotencyKey := getCollectionIdempotencyKey(c)
	if idempotencyKey != "" {
		sourceEventID := events.DeriveNotificationID(
			idempotencyKey,
			"collection.create",
			tenantID,
			req.ContractID,
			req.DebtorID,
			strconv.FormatFloat(req.TotalDebt, 'f', 2, 64),
		)
		coll, err := h.service.CreateIdempotent(c.Request.Context(), req.ContractID, req.DebtorID, req.DebtorName, tenantID, req.TotalDebt, req.DaysOverdue, req.ChargeIDs, sourceEventID)
		if err != nil {
			respondCollectionCreateError(c, err)
			return
		}
		response.Created(c, "Collection created successfully", coll)
		return
	}

	coll, err := h.service.Create(c.Request.Context(), req.ContractID, req.DebtorID, req.DebtorName, tenantID, req.TotalDebt, req.DaysOverdue, req.ChargeIDs)
	if err != nil {
		respondCollectionCreateError(c, err)
		return
	}

	response.Created(c, "Collection created successfully", coll)
}

// respondCollectionCreateError traduz a falha de criação do caso.
//
// A validação das cobranças (F0.7) é erro do chamador, não do servidor: antes
// dela qualquer `charge_ids`/`total_debt` era aceito e o caso nascia com dívida
// inventada. Agora o operador recebe a causa exata.
func respondCollectionCreateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCollectionInput):
		response.ValidationError(c, err.Error())
	case errors.Is(err, service.ErrChargeAlreadyInCase):
		// 409: a dívida já tem caso — abrir outro duplicaria a cobrança.
		response.Conflict(c, err.Error())
	default:
		respondInternalError(c, err)
	}
}

// List godoc
// @Summary      List collections
// @Description  List collection cases with pagination and filters
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        status query string false "Status"
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response{data=[]collection.Collection}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /collections [get]
func (h *CollectionHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	status := c.Query("status")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	collections, total, err := h.service.List(c.Request.Context(), tenantID, status, limit, offset)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Collections retrieved successfully", collections, page, int(limit), total)
}

// GetByID godoc
// @Summary      Get collection by ID
// @Description  Get a collection case by its ID
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Success      200  {object}  response.Response{data=collection.Collection}
// @Failure      404  {object}  response.Response
// @Router       /collections/{id} [get]
func (h *CollectionHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	coll, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Collection retrieved successfully", coll)
}

// Update godoc
// @Summary      Update collection
// @Description  Update an existing collection case
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.UpdateCollectionRequest true "Update Collection Request"
// @Success      200  {object}  response.Response{data=collection.Collection}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /collections/{id} [put]
func (h *CollectionHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateCollectionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if req.DebtorName != "" {
		existing.DebtorName = req.DebtorName
	}
	if req.TotalDebt > 0 {
		existing.TotalDebt = req.TotalDebt
	}
	if req.DaysOverdue > 0 {
		existing.DaysOverdue = req.DaysOverdue
	}
	// Campo ausente significa "não mexe", não "apaga".
	//
	// A atribuição era incondicional e o corpo mais comum desta rota é uma
	// mudança de status (negociar → garantia → judicial) SEM os campos de
	// negociação: cada transição apagava `negotiated_amount`/`negotiated_date`
	// gravados antes. O acordo fechado com o devedor desaparecia do caso, e o
	// valor só voltava se alguém redigitasse.
	if req.NegotiatedAmount != nil {
		existing.NegotiatedAmount = req.NegotiatedAmount
	}
	if req.NegotiatedDate != nil {
		existing.NegotiatedDate = req.NegotiatedDate
	}
	if req.Notes != "" {
		existing.Notes = req.Notes
	}

	actor, _ := context.GetAccountID(c.Request.Context())
	updated, err := h.service.UpdateDetails(c.Request.Context(), existing, req.Status, actor)
	if err != nil {
		switch {
		case errors.Is(err, collection.ErrInvalidStatusTransition), errors.Is(err, service.ErrStatusRequiresDomainData):
			response.Conflict(c, err.Error())
		case errors.Is(err, collection.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		default:
			respondInternalError(c, err)
		}
		return
	}

	response.Success(c, "Collection updated successfully", updated)
}

// AddAction godoc
// @Summary      Add action to collection
// @Description  Add a collection action
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.AddActionRequest true "Add Action Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /collections/{id}/actions [post]
func (h *CollectionHandler) AddAction(c *gin.Context) {
	id := c.Param("id")

	var req AddActionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// Autor da ação: o que o token diz. `req.UserID` só é usado quando o
	// chamador é interno (importação) e mesmo assim fica registrado como veio.
	actor := req.UserID
	if actor == "" {
		actor, _ = context.GetAccountID(c.Request.Context())
	}

	action := collection.Action{
		Date:        time.Now(),
		Type:        req.Type,
		Description: req.Description,
		UserID:      actor,
	}

	if err := h.service.AddAction(c.Request.Context(), tenantID, id, action); err != nil {
		if err.Error() == "collection not found" {
			response.NotFound(c, err.Error())
		} else {
			respondInternalError(c, err)
		}
		return
	}

	// A resposta devolve o caso atualizado: a tela precisa do estado novo (com a
	// ação no histórico) e antes recebia `data: null`, o que a levava a inserir
	// um item nulo na lista local.
	updated, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Action added successfully", updated)
}

// SetJudicial godoc
// @Summary      Register judicial process
// @Description  Registers or updates the judicial process metadata of a collection and escalates it to JUDICIAL
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.JudicialProcessRequest true "Judicial Process Request"
// @Success      200  {object}  response.Response{data=collection.Collection}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /collections/{id}/judicial [post]
func (h *CollectionHandler) SetJudicial(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req JudicialProcessRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	actor, _ := context.GetAccountID(c.Request.Context())
	jp := collection.JudicialProcess{
		CaseNumber: req.CaseNumber,
		Court:      req.Court,
		Lawyer: collection.Lawyer{
			Name:  req.LawyerName,
			OAB:   req.LawyerOAB,
			Email: req.LawyerEmail,
			Phone: req.LawyerPhone,
		},
		ClaimValue:  req.ClaimValue,
		Status:      collection.JudicialStatus(req.Status),
		NextHearing: req.NextHearing,
		Notes:       req.Notes,
	}

	coll, err := h.service.SetJudicialProcess(c.Request.Context(), tenantID, id, jp, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidJudicial):
			response.ValidationError(c, err.Error())
		case errors.Is(err, collection.ErrInvalidStatusTransition), errors.Is(err, collection.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		case err.Error() == "collection not found":
			response.NotFound(c, err.Error())
		default:
			respondInternalError(c, err)
		}
		return
	}

	response.Success(c, "Judicial process registered successfully", coll)
}

// GetJudicial godoc
// @Summary      Get judicial process
// @Description  Returns the judicial process metadata of a collection
// @Tags         collections
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Success      200  {object}  response.Response{data=collection.JudicialProcess}
// @Failure      404  {object}  response.Response
// @Router       /collections/{id}/judicial [get]
func (h *CollectionHandler) GetJudicial(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	coll, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if coll.Judicial == nil {
		response.NotFound(c, "no judicial process registered for this collection")
		return
	}

	response.Success(c, "Judicial process retrieved successfully", coll.Judicial)
}

// ActivateGuarantee godoc
// @Summary      Activate guarantee
// @Description  Activates the rental guarantee (insurance, guarantor or deposit) of a collection and escalates it to GUARANTEE_ACTIVATED
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.ActivateGuaranteeRequest true "Activate Guarantee Request"
// @Success      200  {object}  response.Response{data=collection.Collection}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /collections/{id}/guarantee [post]
func (h *CollectionHandler) ActivateGuarantee(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req ActivateGuaranteeRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	actor, _ := context.GetAccountID(c.Request.Context())
	coll, err := h.service.ActivateGuarantee(c.Request.Context(), tenantID, id, collection.GuaranteeType(req.Type), req.Amount, req.Reason, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidGuarantee):
			response.ValidationError(c, err.Error())
		case errors.Is(err, collection.ErrInvalidStatusTransition), errors.Is(err, collection.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		case err.Error() == "collection not found":
			response.NotFound(c, err.Error())
		default:
			respondInternalError(c, err)
		}
		return
	}

	response.Success(c, "Guarantee activated successfully", coll)
}

// GetGuarantee godoc
// @Summary      Get guarantee activation
// @Description  Returns the guarantee activation record of a collection
// @Tags         collections
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Success      200  {object}  response.Response{data=collection.GuaranteeActivation}
// @Failure      404  {object}  response.Response
// @Router       /collections/{id}/guarantee [get]
func (h *CollectionHandler) GetGuarantee(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	coll, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if coll.Guarantee == nil {
		response.NotFound(c, "no guarantee activation registered for this collection")
		return
	}

	response.Success(c, "Guarantee activation retrieved successfully", coll.Guarantee)
}

// UpdateGuaranteeStatus godoc
// @Summary      Update guarantee activation status
// @Description  Records the guarantor/insurer response on an open guarantee activation
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.UpdateGuaranteeStatusRequest true "Update Guarantee Status Request"
// @Success      200  {object}  response.Response{data=collection.Collection}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /collections/{id}/guarantee/status [put]
func (h *CollectionHandler) UpdateGuaranteeStatus(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateGuaranteeStatusRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	actor, _ := context.GetAccountID(c.Request.Context())

	// Baixa por garantia: PAID COM valor recebido é o caminho que efetivamente
	// encerra a dívida (aplica o valor nas cobranças, credita o excedente e
	// resolve o caso quando quita tudo). PAID sem valor continua sendo só o
	// registro do status — mantido para não quebrar clientes antigos.
	if collection.GuaranteeActivationStatus(req.Status) == collection.GuaranteeStatusPaid && req.SettledAmount > 0 {
		coll, resultado, err := h.service.SettleGuarantee(c.Request.Context(), tenantID, id, req.SettledAmount, req.SettlementReference, req.InsurerResponse, actor)
		if err != nil {
			h.respondGuaranteeError(c, err)
			return
		}
		response.Success(c, "Garantia quitada e cobranças baixadas", map[string]any{
			"collection": coll,
			"settlement": resultado,
		})
		return
	}

	coll, err := h.service.UpdateGuaranteeStatus(c.Request.Context(), tenantID, id, collection.GuaranteeActivationStatus(req.Status), req.InsurerResponse, actor)
	if err != nil {
		h.respondGuaranteeError(c, err)
		return
	}

	// Recusa/cancelamento devolve o caso à cobrança (a régua retoma).
	if coll != nil && (collection.GuaranteeActivationStatus(req.Status) == collection.GuaranteeStatusRejected ||
		collection.GuaranteeActivationStatus(req.Status) == collection.GuaranteeStatusCancelled) {
		if reaberto, err := h.service.ReopenAfterGuaranteeRejection(c.Request.Context(), tenantID, id, actor); err == nil && reaberto != nil {
			coll = reaberto
		}
	}

	response.Success(c, "Guarantee activation updated successfully", coll)
}

func (h *CollectionHandler) respondGuaranteeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidGuarantee), errors.Is(err, service.ErrInvalidSettlement):
		response.ValidationError(c, err.Error())
	case errors.Is(err, collection.ErrConcurrentModification), errors.Is(err, collection.ErrInvalidStatusTransition):
		response.Conflict(c, err.Error())
	case err.Error() == "collection not found":
		response.NotFound(c, err.Error())
	default:
		respondInternalError(c, err)
	}
}

// ── Fase 2: acordo, desfecho judicial e baixa por garantia ──────────────────

// CreateAgreementRequest — corpo de POST /collections/:id/agreement.
type CreateAgreementRequest struct {
	TotalAmount  float64 `json:"total_amount" binding:"required,gt=0"`
	Discount     float64 `json:"discount"`
	DownPayment  float64 `json:"down_payment"`
	Installments int     `json:"installments" binding:"required"`
	IntervalDays int     `json:"interval_days"`
	FirstDueDate string  `json:"first_due_date" binding:"required"`
	Notes        string  `json:"notes"`
}

// AgreementActionRequest — corpo de break/cancel do acordo.
type AgreementActionRequest struct {
	Reason string `json:"reason" binding:"required"`
	// RestoreDiscount recria o desconto concedido como cobrança nova. Desligado
	// por padrão: criar dívida é decisão do operador.
	RestoreDiscount bool `json:"restore_discount"`
}

// JudicialOutcomeRequest — corpo de POST /collections/:id/judicial/outcome.
type JudicialOutcomeRequest struct {
	Status    string                    `json:"status" binding:"required"`
	DecidedAt *time.Time                `json:"decided_at,omitempty"`
	Notes     string                    `json:"notes"`
	Costs     []collection.JudicialCost `json:"costs,omitempty"`
}

// CreateAgreement godoc
// @Summary      Formaliza o acordo do caso
// @Description  Cria o acordo, gera as parcelas como cobranças e substitui as cobranças originais
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.CreateAgreementRequest true "Agreement Request"
// @Success      201  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      409  {object}  response.Response
// @Router       /collections/{id}/agreement [post]
func (h *CollectionHandler) CreateAgreement(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req CreateAgreementRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	primeiroVencimento, err := parseFlexibleDate(req.FirstDueDate)
	if err != nil {
		response.ValidationError(c, "first_due_date inválido: use AAAA-MM-DD ou RFC3339")
		return
	}
	intervalo := req.IntervalDays
	if intervalo == 0 {
		intervalo = 30
	}

	actor, _ := context.GetAccountID(c.Request.Context())
	podeAprovar := h.podeAprovarDesconto(c)

	caso, acordo, err := h.service.CreateAgreement(c.Request.Context(), tenantID, id, collection.NewAgreementInput{
		TotalAmount:  req.TotalAmount,
		Discount:     req.Discount,
		DownPayment:  req.DownPayment,
		Installments: req.Installments,
		IntervalDays: intervalo,
		FirstDueDate: primeiroVencimento,
		Notes:        req.Notes,
	}, actor, podeAprovar)
	if err != nil {
		h.respondAgreementError(c, err)
		return
	}

	response.Created(c, "Acordo registrado com sucesso", map[string]any{
		"collection": caso,
		"agreement":  acordo,
	})
}

// GetAgreement godoc
// @Summary      Consulta o acordo vigente do caso
// @Tags         collections
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Success      200  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /collections/{id}/agreement [get]
func (h *CollectionHandler) GetAgreement(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	acordo, err := h.service.GetAgreement(c.Request.Context(), tenantID, id)
	if err != nil {
		if err.Error() == "collection not found" {
			response.NotFound(c, err.Error())
			return
		}
		respondInternalError(c, err)
		return
	}
	if acordo == nil {
		response.NotFound(c, "caso não possui acordo vigente")
		return
	}
	response.Success(c, "Acordo recuperado com sucesso", acordo)
}

// BreakAgreement godoc
// @Summary      Quebra o acordo e devolve o caso à cobrança
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.AgreementActionRequest true "Motivo"
// @Success      200  {object}  response.Response
// @Router       /collections/{id}/agreement/break [post]
func (h *CollectionHandler) BreakAgreement(c *gin.Context) {
	h.endAgreement(c, true)
}

// CancelAgreement godoc
// @Summary      Cancela o acordo sem tratar como quebra
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.AgreementActionRequest true "Motivo"
// @Success      200  {object}  response.Response
// @Router       /collections/{id}/agreement/cancel [post]
func (h *CollectionHandler) CancelAgreement(c *gin.Context) {
	h.endAgreement(c, false)
}

func (h *CollectionHandler) endAgreement(c *gin.Context, quebra bool) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req AgreementActionRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	actor, _ := context.GetAccountID(c.Request.Context())

	var caso *collection.Collection
	var acordo *collection.Agreement
	if quebra {
		caso, acordo, err = h.service.BreakAgreement(c.Request.Context(), tenantID, id, req.Reason, actor, req.RestoreDiscount)
	} else {
		caso, acordo, err = h.service.CancelAgreement(c.Request.Context(), tenantID, id, req.Reason, actor)
	}
	if err != nil {
		h.respondAgreementError(c, err)
		return
	}
	mensagem := "Acordo cancelado"
	if quebra {
		mensagem = "Acordo quebrado: o caso voltou para cobrança ativa"
	}
	response.Success(c, mensagem, map[string]any{"collection": caso, "agreement": acordo})
}

// RegisterJudicialOutcome godoc
// @Summary      Registra o desfecho do processo judicial
// @Description  Aplica o efeito do desfecho no caso e lança as custas como despesa
// @Tags         collections
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Collection ID"
// @Param        request body handlers.JudicialOutcomeRequest true "Desfecho"
// @Success      200  {object}  response.Response
// @Router       /collections/{id}/judicial/outcome [post]
func (h *CollectionHandler) RegisterJudicialOutcome(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req JudicialOutcomeRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	actor, _ := context.GetAccountID(c.Request.Context())

	var decididoEm time.Time
	if req.DecidedAt != nil {
		decididoEm = *req.DecidedAt
	}
	caso, err := h.service.RegisterJudicialOutcome(c.Request.Context(), tenantID, id,
		collection.JudicialStatus(req.Status), decididoEm, req.Notes, req.Costs, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidJudicialOutcome), errors.Is(err, service.ErrInvalidJudicial):
			response.ValidationError(c, err.Error())
		case errors.Is(err, collection.ErrInvalidStatusTransition), errors.Is(err, collection.ErrConcurrentModification):
			response.Conflict(c, err.Error())
		case err.Error() == "collection not found":
			response.NotFound(c, err.Error())
		default:
			respondInternalError(c, err)
		}
		return
	}
	response.Success(c, "Desfecho judicial registrado", caso)
}

func (h *CollectionHandler) respondAgreementError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, collection.ErrInvalidAgreement):
		response.ValidationError(c, err.Error())
	case errors.Is(err, service.ErrApprovalRequired):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrAgreementConflict):
		response.Conflict(c, err.Error())
	case errors.Is(err, collection.ErrAgreementNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, collection.ErrAgreementAlreadyEnded), errors.Is(err, collection.ErrInvalidStatusTransition),
		errors.Is(err, collection.ErrConcurrentModification):
		response.Conflict(c, err.Error())
	case err.Error() == "collection not found":
		response.NotFound(c, err.Error())
	default:
		respondInternalError(c, err)
	}
}

// podeAprovarDesconto reusa o gate M11 das cobranças: a alçada de desconto é a
// mesma permissão dedicada (`finance:approve_adjustment`) e falha FECHADO — sem
// provider configurado ninguém aprova, em vez de todo mundo aprovar.
func (h *CollectionHandler) podeAprovarDesconto(c *gin.Context) bool {
	return actorCanApproveAdjustment(c, h.permProvider)
}

// parseFlexibleDate aceita AAAA-MM-DD (o que um formulário manda) e RFC3339.
func parseFlexibleDate(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, errors.New("empty date")
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", v)
}

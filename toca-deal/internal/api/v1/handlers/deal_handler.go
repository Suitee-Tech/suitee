package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/context"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// normalizeIdempotencyKey garante UTF-8 válido antes do banco.
//
// Navegadores codificam valores de header como Latin-1 (ISO-8859-1 — RFC 9110
// §5.5): uma chave com "ção" chega como bytes 0xe7 0xe3 0x6f, e o Postgres
// rejeita o texto com SQLSTATE 22021 no lookup/gravação de
// metadata->>'idempotency_key'. Chaves já em UTF-8 válido passam intactas;
// bytes inválidos são reinterpretados como Latin-1 (cada byte vira o code
// point correspondente) — determinístico para o mesmo cliente reenviar.
func normalizeIdempotencyKey(key string) string {
	if key == "" || utf8.ValidString(key) {
		return key
	}
	raw := []byte(key)
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes)
}

// Force usage of deal package for Swagger
var _ = deal.Deal{}

type DealHandler struct {
	service            *service.DealService
	contractService    *service.ContractService
	permissionProvider auth.PermissionProvider
}

func NewDealHandler(service *service.DealService) *DealHandler {
	return &DealHandler{service: service}
}

// WithContractService is optional wiring used by the client portal signature
// endpoint, which needs to cross domains (deal access check + contract +
// envelope projection). Kept as a setter so other call sites don't have to
// thread an extra dep.
func (h *DealHandler) WithContractService(cs *service.ContractService) *DealHandler {
	h.contractService = cs
	return h
}

// WithPermissionProvider wires the permission catalog used by the
// approval gate (#416) in UpdateStatus. Optional — quando ausente, o
// gate degrada para "approval sempre permitido" (compatível com testes
// e dev local). Em produção routes.go injeta o CorePermissionProvider.
func (h *DealHandler) WithPermissionProvider(p auth.PermissionProvider) *DealHandler {
	h.permissionProvider = p
	return h
}

func (h *DealHandler) scopedBrokerID(c *gin.Context) (string, string, error) {
	accountID, err := context.GetAccountID(c.Request.Context())
	if err != nil {
		return "", "", err
	}
	// P1/A25: a regra de "quem não é admin/owner/manager é recortado pelo próprio
	// account id" vive em callerBrokerScope (broker_scope.go), compartilhada com os
	// handlers de contrato e documento.
	return callerBrokerScope(c), accountID, nil
}

type CreateDealRequest struct {
	Type              deal.Type `json:"type" binding:"required"`
	PropertyID        string    `json:"property_id" binding:"required"`
	OwnerID           string    `json:"owner_id" binding:"required"`
	CustomerID        string    `json:"customer_id" binding:"required"` // Lead/Client ID (buyer or tenant)
	LeadID            string    `json:"lead_id,omitempty"`              // Explicit lead reference (optional, defaults to CustomerID)
	BrokerID          string    `json:"broker_id" binding:"required"`
	Value             float64   `json:"value" binding:"required,gt=0"`
	CustomerName      string    `json:"customer_name,omitempty"`
	CustomerEmail     string    `json:"customer_email,omitempty"`
	CustomerAccountID string    `json:"customer_account_id,omitempty"`
	CustomerRole      string    `json:"customer_role,omitempty"` // e.g., "comprador", "locatario", "inquilino"
	// Owner identity hints — when the caller already has the property owner's
	// lead profile loaded (e.g. the captação leads list in the CRM), passing
	// these fields lets us populate the owner Party directly instead of relying
	// on a service-to-service CRM lookup that may fail silently (missing
	// INTERNAL_API_KEY, CRM_SERVICE_URL, network).
	OwnerName            string                       `json:"owner_name,omitempty"`
	OwnerEmail           string                       `json:"owner_email,omitempty"`
	OwnerPhone           string                       `json:"owner_phone,omitempty"`
	OwnerDocument        string                       `json:"owner_document,omitempty"`
	OwnerAccountID       string                       `json:"owner_account_id,omitempty"`
	Parties              []CreatePartyRequest         `json:"parties,omitempty"`
	BrokerParticipations []SetBrokerParticipationItem `json:"broker_participations,omitempty" binding:"omitempty,dive"`
}

type CreatePartyRequest struct {
	LeadID           string                    `json:"lead_id,omitempty"`
	AccountID        string                    `json:"account_id,omitempty"`
	Name             string                    `json:"name,omitempty"`
	Email            string                    `json:"email,omitempty"`
	Phone            string                    `json:"phone,omitempty"`
	Document         string                    `json:"document,omitempty"`
	Kind             deal.PartyKind            `json:"kind,omitempty"`
	Role             deal.PartyRole            `json:"role" binding:"required"`
	SpouseOfPartyID  string                    `json:"spouse_of_party_id,omitempty"`
	NoSpouseDeclared bool                      `json:"no_spouse_declared,omitempty"`
	Representation   *deal.PartyRepresentation `json:"representation,omitempty"`
	SharePct         float64                   `json:"share_pct,omitempty"`
	IsPrimary        bool                      `json:"is_primary,omitempty"`
}

type UpdateStatusRequest struct {
	Status deal.Status `json:"status" binding:"required"`
	Notes  string      `json:"notes"`
}

// Create godoc
// @Summary      Create deal
// @Description  Create a new deal
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateDealRequest true "Create Deal Request"
// @Success      201  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals [post]
// dealAttributionRequiresAuthority informa se a atribuição pedida precisa de
// autoridade extra (`deals:approve`).
//
// SECURITY (achado A24): atribuir o negócio a SI MESMO é o fluxo normal do
// corretor (não exige nada além de `deals:write`); atribuir a OUTRA pessoa —
// ou quando não há conta no contexto — exige autoridade de gestão, porque a
// atribuição dirige comissões e o escopo aplicado nas demais rotas.
func dealAttributionRequiresAuthority(c *gin.Context, brokerID string) bool {
	if strings.TrimSpace(brokerID) == "" {
		return false
	}
	actorID, _ := pkgcontext.GetAccountID(c.Request.Context())
	if actorID == "" {
		return true
	}
	return brokerID != actorID
}

// participationsIncludeActorOrAgency diz se a divisão de comissão pedida mantém
// o chamador (ou a própria imobiliária) entre os beneficiários.
//
// N14 (reauditoria de 2026-09-20): é a guarda barata para o caso em que o
// chamador NÃO precisou de `deals:approve` — ele está criando o negócio no
// próprio nome, então não pode sumir da divisão que ele mesmo define.
func participationsIncludeActorOrAgency(items []SetBrokerParticipationItem, actorID string) bool {
	for _, p := range items {
		if p.Role == deal.BrokerRoleImobiliaria {
			return true
		}
		if actorID != "" && strings.TrimSpace(p.BrokerID) == actorID {
			return true
		}
	}
	return false
}

// commissionSplitExcludesResponsibleBroker diz se a divisão de comissão pedida
// exclui o próprio responsável SEM que o chamador tenha autoridade de gestão.
//
// N14 (reauditoria de 2026-09-20): participações nomeando terceiros são
// `deals:write` por decisão de produto — a rota irmã
// (`PUT /deals/:id/broker-participations`, mesma permissão) já deixa o
// responsável montar a divisão do PRÓPRIO negócio. O que faltava era a checagem
// de que o responsável CONTINUA na divisão: sem ela um `corretor` criava o
// negócio no próprio nome (passa no gate de atribuição, sem autoridade) e
// roteava 100% da comissão para ids arbitrários. Quem precisou de
// `deals:approve` para atribuir (admin/gestor/operações) segue livre para montar
// a divisão sem si mesmo — é o caso legítimo do gestor que redistribui.
func commissionSplitExcludesResponsibleBroker(c *gin.Context, items []SetBrokerParticipationItem, requiresAttributionAuthority bool) bool {
	if len(items) == 0 || requiresAttributionAuthority {
		return false
	}
	actorID, _ := pkgcontext.GetAccountID(c.Request.Context())
	if actorID == "" {
		return false
	}
	return !participationsIncludeActorOrAgency(items, actorID)
}

func (h *DealHandler) Create(c *gin.Context) {
	var req CreateDealRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	leadID := req.LeadID
	if leadID == "" {
		leadID = req.CustomerID // default: lead ID = customer ID
	}

	// SECURITY (achado A24 da revisão de permissões de 2026-09-19): a criação
	// aceitava `broker_id` de terceiro sem nenhuma checagem — um corretor criava
	// negócio atribuído a outro, e a atribuição dirige a distribuição de
	// comissões (`broker_participations`) e o escopo que as demais rotas
	// aplicam. Atribuir a OUTRA pessoa exige `deals:approve`
	// (admin/gestor/operacoes), o mesmo conjunto que move negócios no gate de
	// valor.
	requiresAttributionAuthority := dealAttributionRequiresAuthority(c, req.BrokerID)
	if requiresAttributionAuthority {
		if h.permissionProvider == nil || !auth.HasPermission(c, h.permissionProvider, "deals:approve") {
			response.Forbidden(c, "permission denied: deals:approve is required to assign the deal to another broker")
			return
		}
	}

	// N14 (reauditoria de 2026-09-20): ver a explicação em
	// `commissionSplitExcludesResponsibleBroker`.
	if commissionSplitExcludesResponsibleBroker(c, req.BrokerParticipations, requiresAttributionAuthority) {
		response.Forbidden(c, "permission denied: deals:approve is required to exclude the responsible broker from the commission split")
		return
	}

	ownerHints := service.PartyProfileHints{
		Name:      req.OwnerName,
		Email:     req.OwnerEmail,
		Phone:     req.OwnerPhone,
		Document:  req.OwnerDocument,
		AccountID: req.OwnerAccountID,
	}

	// Co-brokering gate: when the deal references a property/lead owned by a
	// partner company, the service takes the cross-tenant branch and requires
	// the caller to hold `shared_content:operate`. We evaluate the permission
	// here (the service is HTTP-agnostic) and hand it down — the service only
	// enforces it when the cross-tenant branch is actually taken, so a normal
	// same-tenant deal is never blocked by a missing shared_content:operate.
	allowCrossTenantOperate := false
	if h.permissionProvider != nil {
		allowCrossTenantOperate = auth.HasPermission(c, h.permissionProvider, "shared_content:operate")
	}

	// Idempotency-Key identifica a intenção de criação: reenvio do mesmo
	// formulário (duplo clique/retry) devolve o negócio já criado em vez de
	// duplicar. Clientes sem a chave são protegidos pela guarda de janela.
	idempotencyKey := normalizeIdempotencyKey(strings.TrimSpace(c.GetHeader("Idempotency-Key")))

	var d *deal.Deal
	if len(req.Parties) > 0 {
		parties := make([]deal.Party, 0, len(req.Parties))
		for _, p := range req.Parties {
			role := p.Role
			if req.Type == deal.TypeRent && role == deal.PartyRoleBuyer {
				role = deal.PartyRoleTenant
			}
			parties = append(parties, deal.Party{
				LeadID:           p.LeadID,
				AccountID:        p.AccountID,
				Name:             p.Name,
				Email:            p.Email,
				Phone:            p.Phone,
				Document:         p.Document,
				Kind:             p.Kind,
				Role:             role,
				SharePct:         p.SharePct,
				IsPrimary:        p.IsPrimary,
				NoSpouseDeclared: p.NoSpouseDeclared,
			})
		}
		d, err = h.service.CreateWithParties(
			c.Request.Context(),
			req.Type,
			req.PropertyID,
			req.OwnerID,
			req.CustomerID,
			req.BrokerID,
			leadID,
			req.Value,
			tenantID,
			parties,
			ownerHints,
			allowCrossTenantOperate,
			service.WithIdempotencyKey(idempotencyKey),
		)
	} else {
		d, err = h.service.Create(
			c.Request.Context(),
			req.Type,
			req.PropertyID,
			req.OwnerID,
			req.CustomerID,
			req.BrokerID,
			leadID,
			req.Value,
			tenantID,
			req.CustomerName,
			req.CustomerEmail,
			req.CustomerAccountID,
			req.CustomerRole,
			ownerHints,
			allowCrossTenantOperate,
			service.WithIdempotencyKey(idempotencyKey),
		)
	}
	if err != nil {
		var dupErr *service.DuplicateDealError
		switch {
		case errors.As(err, &dupErr):
			// 409 com o negócio existente: o painel aponta o cadastro em vez de
			// criar um segundo negócio com as mesmas partes, imóvel e valor.
			c.JSON(http.StatusConflict, response.Response{
				Success:    false,
				Message:    dupErr.Error(),
				StatusCode: http.StatusConflict,
				Data:       gin.H{"existing_deal_id": dupErr.ExistingID},
			})
		case strings.HasPrefix(err.Error(), "invalid party role") || strings.HasPrefix(err.Error(), "invalid signatory role"):
			response.ValidationError(c, err.Error())
		case errors.Is(err, service.ErrCrossTenantResourceDenied), errors.Is(err, service.ErrSharedContentOperateDenied):
			response.Forbidden(c, err.Error())
		case validation.IsPersonNameError(err):
			// Nome de parte sem sobrenome (lead antigo) é dado a corrigir: o
			// operador precisa do campo, não de um erro interno.
			response.ValidationError(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}
	if len(req.BrokerParticipations) > 0 {
		participations := make([]deal.BrokerParticipation, 0, len(req.BrokerParticipations))
		for _, p := range req.BrokerParticipations {
			participations = append(participations, deal.BrokerParticipation{
				BrokerID:   p.BrokerID,
				Role:       deal.BrokerParticipationRole(p.Role),
				Percentage: p.Percentage,
			})
		}
		d, err = h.service.SetBrokerParticipations(
			c.Request.Context(), tenantID, d.ID, "", req.BrokerID, "", nil, participations,
		)
		if err != nil {
			response.ValidationError(c, err.Error())
			return
		}
	}

	response.Created(c, "Deal created successfully", d)
}

// List godoc
// @Summary      List deals
// @Description  List deals with pagination
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        limit query int false "Limit"
// @Param        offset query int false "Offset"
// @Success      200  {object}  response.Response{data=[]deal.Deal}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals [get]
func (h *DealHandler) List(c *gin.Context) {
	limit, err := strconv.ParseInt(c.DefaultQuery("limit", "10"), 10, 64)
	if err != nil || limit <= 0 {
		limit = 10
	}
	offset, err := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	if err != nil || offset < 0 {
		offset = 0
	}

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

	customerIDs := make([]string, 0)
	if v := c.Query("customer_id"); v != "" {
		customerIDs = append(customerIDs, strings.TrimSpace(v))
	}
	if v := c.Query("customer_ids"); v != "" {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				customerIDs = append(customerIDs, part)
			}
		}
	}
	if len(customerIDs) > 0 {
		seen := map[string]struct{}{}
		uniq := make([]string, 0, len(customerIDs))
		for _, id := range customerIDs {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			uniq = append(uniq, id)
		}
		customerIDs = uniq
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
		if t, err := time.Parse("2006-01-02", v); err == nil {
			return &t, nil
		}
		return nil, fmt.Errorf("invalid time")
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

	createdFrom, err := parseTime(c.Query("created_from"))
	if err != nil {
		response.ValidationError(c, "invalid created_from value")
		return
	}
	createdTo, err := parseTime(c.Query("created_to"))
	if err != nil {
		response.ValidationError(c, "invalid created_to value")
		return
	}
	minValue, err := parseFloatPtr(c.Query("min_value"))
	if err != nil {
		response.ValidationError(c, "invalid min_value value")
		return
	}
	maxValue, err := parseFloatPtr(c.Query("max_value"))
	if err != nil {
		response.ValidationError(c, "invalid max_value value")
		return
	}

	var dealStatus deal.Status
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		dealStatus = deal.Status(strings.ToUpper(v))
	}
	var dealType deal.Type
	if v := strings.TrimSpace(c.Query("type")); v != "" {
		switch strings.ToLower(v) {
		case "sale", "venda", "vend":
			dealType = deal.TypeSale
		case "rent", "locacao", "locação":
			dealType = deal.TypeRent
		default:
			dealType = deal.Type(strings.ToUpper(v))
		}
	}

	filter := deal.DealFilter{
		Limit:  limit,
		Offset: offset,

		Status: dealStatus,
		Type:   dealType,

		MinValue: minValue,
		MaxValue: maxValue,

		PropertyID:      c.Query("property_id"),
		PropertyAddress: c.Query("property_address"),

		PartyName:  c.Query("party_name"),
		PartyEmail: c.Query("party_email"),
		LeadID:     c.Query("lead_id"),

		BrokerID: c.Query("broker_id"),

		CreatedFrom: createdFrom,
		CreatedTo:   createdTo,

		Search: c.Query("search"),

		CustomerIDs: customerIDs,
	}

	deals, total, err := h.service.Search(c.Request.Context(), tenantID, filter, brokerID)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Deals retrieved successfully", []*deal.Deal{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Deals retrieved successfully", deals, page, int(limit), total)
}

// GetStats godoc
// @Summary      Get deal stats
// @Description  Get active proposals count and monthly sales value
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /deals/stats [get]
func (h *DealHandler) GetStats(c *gin.Context) {
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

	proposals, sales, err := h.service.GetStats(c.Request.Context(), tenantID, brokerID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Stats retrieved successfully", gin.H{
				"active_proposals": 0,
				"monthly_sales":    0,
			})
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Stats retrieved successfully", gin.H{
		"active_proposals": proposals,
		"monthly_sales":    sales,
	})
}

// GetByID godoc
// @Summary      Get deal by ID
// @Description  Get a deal by its ID
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      404  {object}  response.Response
// @Router       /deals/{id} [get]
func (h *DealHandler) GetByID(c *gin.Context) {
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
	d, err := h.service.GetByID(c.Request.Context(), tenantID, id, brokerID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	// Populate the computed checklist snapshot before serialization so
	// the Esteira Documentation tab can render progress without an
	// extra round-trip. See deal.ChecklistStats.
	d.PopulateChecklistStats()
	response.Success(c, "Deal retrieved successfully", d)
}

// GetByIDInternal is the internal-API variant of GetByID consumed by
// other Toca services (notably toca-financial's FinancialPreviewService).
// It bypasses the broker scope check because internal callers don't
// carry user context — authorization is enforced upstream by the
// internal API key middleware. tenant_id is required as a query string.
func (h *DealHandler) GetByIDInternal(c *gin.Context) {
	id := c.Param("id")
	// A3 (reauditoria de 2026-09-20): o tenant vinha cru do `?tenant_id=`. A rota
	// é autenticada por credencial de serviço, mas nada prendia o tenant pedido à
	// credencial — uma chave nomeada com allowlist do tenant A lia o tenant B.
	// Resolvedor único, já adotado em signature/financial/crm/comm/calendar/ops.
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
	d, err := h.service.GetByID(c.Request.Context(), tenantID, id, "")
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	d.PopulateChecklistStats()
	response.Success(c, "Deal retrieved successfully", d)
}

// GetAuditLog godoc
// @Summary      Get audit trail for a deal
// @Description  Return the audit.events entries for this deal, newest first.
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        limit query int false "Page size (default 100, max 500)"
// @Success      200  {object}  response.Response{data=[]audit.LogEntry}
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /deals/{id}/audit-log [get]
func (h *DealHandler) GetAuditLog(c *gin.Context) {
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
	// Re-use the existing access check: if the requester can't see the deal,
	// they can't see its audit trail either. GetByID returns NotFound on a
	// permission miss, which is what we want — leaks no existence info.
	if _, err := h.service.GetByID(c.Request.Context(), tenantID, id, brokerID); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, perr := strconv.Atoi(raw); perr == nil {
			limit = n
		}
	}

	entries, err := audit.ListByEntity(c.Request.Context(), tenantID, "Deal", id, limit)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Audit log retrieved", entries)
}

// UpdateStatus godoc
// @Summary      Update deal status
// @Description  Update the status of a deal
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.UpdateStatusRequest true "Update Status Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/status [patch]

// actorCanApproveDeal decide se o chamador pode aprovar a transição de um deal
// que exige aprovação por valor.
//
// SECURITY (achado A23): falha FECHADO. Sem provider de permissão configurado a
// resposta é "não pode aprovar" — o oposto do comportamento anterior, que
// tratava provider ausente como autorização concedida. `auth.HasPermission` já
// falha fechado para tenant ausente, papel ausente e erro do provider; o único
// ponto permissivo era este default.
func actorCanApproveDeal(c *gin.Context, provider auth.PermissionProvider) bool {
	if provider == nil {
		return false
	}
	return auth.HasPermission(c, provider, "deals:approve")
}

func (h *DealHandler) UpdateStatus(c *gin.Context) {
	id := c.Param("id")
	var req UpdateStatusRequest
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

	// Approval gate (#416): apenas perfis com `deals:approve` movem deals
	// acima do threshold (R$ 500k default). Avalia AGORA, no handler, em
	// vez de no middleware porque a regra depende do valor do deal — não
	// pode ser uma decisão estática por rota.
	//
	// SECURITY (achado A23 da revisão de permissões de 2026-09-19): antes o
	// default era `actorCanApprove := true` quando o provider não estava wired —
	// uma regressão de wiring (provider nil, cenário que o próprio comentário
	// previa) desligava em SILÊNCIO a exigência de aprovação por valor. O gate
	// agora falha FECHADO.
	actorCanApprove := actorCanApproveDeal(c, h.permissionProvider)

	d, err := h.service.UpdateStatusAuthorized(c.Request.Context(), tenantID, id, brokerID, req.Status, userID, req.Notes, actorCanApprove)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrApprovalRequired):
			response.Forbidden(c, err.Error())
			return
		case errors.Is(err, service.ErrProposalNotApproved),
			errors.Is(err, service.ErrPartiesPending),
			errors.Is(err, service.ErrSpouseMissing),
			errors.Is(err, service.ErrChecklistNotApproved),
			errors.Is(err, service.ErrSignersNotComplete):
			response.ValidationError(c, err.Error())
			return
		}
		if err.Error() == "invalid status transition" {
			response.ValidationError(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Deal status updated successfully", d)
}

// ReconcileSigners godoc
// @Summary      Force-resync the signer projection for a deal
// @Description  Forces the deal.Signers projection to be re-applied from
// @Description  the contract's signatories. Recovery hatch for the
// @Description  Esteira "Reconciliar" button when the periodic event flow
// @Description  has drifted. Idempotent.
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "Deal ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/reconcile-signers [post]
func (h *DealHandler) ReconcileSigners(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		response.ValidationError(c, "deal id is required")
		return
	}
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	// P1/A25: o negócio é resolvido com o escopo do chamador — um corretor não
	// ressincroniza signatários de negócio de outro corretor.
	brokerScope := callerBrokerScope(c)
	if err := h.service.ReconcileSigners(c.Request.Context(), tenantID, id, brokerScope); err != nil {
		if errors.Is(err, service.ErrDealNotInScope) {
			response.NotFound(c, "deal not found")
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Signer projection reconciled", gin.H{"deal_id": id, "tenant_id": tenantID})
}

// Delete godoc
// @Summary      Delete deal
// @Description  Delete a deal by its ID
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Success      200  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id} [delete]
func (h *DealHandler) Delete(c *gin.Context) {
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
	if err := h.service.Delete(c.Request.Context(), tenantID, id, brokerID); err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Deal deleted successfully", nil)
}

type UpdateDueDiligenceRequest struct {
	ScoreSerasa   *int   `json:"score_serasa"`
	CNDStatus     string `json:"cnd_status"`
	LegalOpinion  string `json:"legal_opinion"`
	MonthlyIncome *int   `json:"monthly_income"`
	Status        string `json:"status"`
}

// UpdateDueDiligence godoc
// @Summary      Update due diligence
// @Description  Update due diligence information
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.UpdateDueDiligenceRequest true "Update Due Diligence Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/due-diligence [put]
func (h *DealHandler) UpdateDueDiligence(c *gin.Context) {
	id := c.Param("id")
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

	d, err := h.service.UpdateDueDiligence(c.Request.Context(), tenantID, id, brokerID, req.ScoreSerasa, req.CNDStatus, req.LegalOpinion, req.MonthlyIncome, req.Status, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Due diligence updated successfully", d)
}

// BackfillChecklistDocuments godoc
// @Summary      Materializa no painel de documentos os arquivos da esteira digital
// @Description  Varre os negócios do tenant e cria/atualiza a linha de documento
// @Description  de cada item de checklist já enviado pelo cliente. Idempotente.
// @Tags         documents
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=service.ChecklistDocumentBackfillStats}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/backfill-checklist-documents [post]
func (h *DealHandler) BackfillChecklistDocuments(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	stats, err := h.service.BackfillChecklistDocuments(c.Request.Context(), tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Checklist documents backfilled", stats)
}

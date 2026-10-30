package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-crm/internal/api/v1/dto"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/service"
)

// Force usage of lead package for Swagger
var _ = lead.Lead{}

const (
	// DefaultLeadListLimit is returned when the client omits ?limit.
	DefaultLeadListLimit int64 = 25
	// MaxLeadListLimit caps the maximum number of leads returned in a single list call.
	// Tenant-scope queries are further restricted to MaxTenantScopeLimit.
	MaxLeadListLimit int64 = 100
	// MaxTenantScopeLimit caps tenant-wide listings to keep payloads bounded.
	MaxTenantScopeLimit int64 = 10
)

// ParseLeadListLimit normalizes a client-supplied ?limit value for the /leads
// list endpoint. An empty raw value yields the default. Values below 1 or
// above MaxLeadListLimit produce a non-empty error message and a zero limit.
// Tenant-scope queries are silently capped at MaxTenantScopeLimit.
func ParseLeadListLimit(scope, raw string) (int64, string) {
	if raw == "" {
		limit := DefaultLeadListLimit
		if scope == "tenant" && limit > MaxTenantScopeLimit {
			limit = MaxTenantScopeLimit
		}
		return limit, ""
	}
	limit, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || limit < 1 {
		return 0, "invalid limit value"
	}
	if limit > MaxLeadListLimit {
		return 0, fmt.Sprintf("limit exceeds maximum of %d", MaxLeadListLimit)
	}
	if scope == "tenant" && limit > MaxTenantScopeLimit {
		limit = MaxTenantScopeLimit
	}
	return limit, ""
}

type LeadHandler struct {
	service            *service.LeadService
	rouletteService    *service.LeadRouletteService
	slaService         *service.LeadSLAService
	permissionProvider auth.PermissionProvider
}

// WithPermissionProvider injeta o provider do catálogo. Usado para decisões que
// dependem do RECURSO (não são estáticas por rota) — hoje a posse na leitura de
// lead (achado A10). Em produção routes.go injeta o CorePermissionProvider.
func (h *LeadHandler) WithPermissionProvider(p auth.PermissionProvider) *LeadHandler {
	h.permissionProvider = p
	return h
}

func NewLeadHandler(service *service.LeadService, rouletteService *service.LeadRouletteService, slaService *service.LeadSLAService) *LeadHandler {
	return &LeadHandler{
		service:         service,
		rouletteService: rouletteService,
		slaService:      slaService,
	}
}

type CreateLeadRequest struct {
	Name             string             `json:"name" binding:"required"`
	Email            string             `json:"email" binding:"required,email"`
	Phone            string             `json:"phone" binding:"required"`
	Avatar           string             `json:"avatar"`
	Source           lead.LeadSource    `json:"source" binding:"required"`
	Status           lead.LeadStatus    `json:"status"`
	PropertyInterest *lead.LeadProperty `json:"property_interest"`
	// Recomendação vinda da vitrine: o lead já nasce apontando para o imóvel
	// que o comprador abriu. O snapshot evita uma leitura cross-tenant no
	// portal do cliente.
	RecommendedPropertyID       string                            `json:"recommended_property_id,omitempty"`
	RecommendedPropertySnapshot *lead.RecommendedPropertySnapshot `json:"recommended_property_snapshot,omitempty"`
	AssignedBrokerID            string                            `json:"assigned_broker_id"`
	Qualification               map[string]interface{}            `json:"qualification,omitempty"`
	// Client segmentation
	Role     lead.ClientRole `json:"role,omitempty"`
	IsClient bool            `json:"is_client,omitempty"`
	// Captação link
	CaptacaoPropertyID string `json:"captacao_property_id,omitempty"`
	OwnerLeadID        string `json:"owner_lead_id,omitempty"`
	// PartnershipShared opts this lead into the partner shared-content feature.
	PartnershipShared bool `json:"partnership_shared,omitempty"`
}

// Create godoc
// @Summary      Create lead
// @Description  Create a new lead
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateLeadRequest true "Create Lead Request"
// @Success      201  {object}  response.Response{data=lead.Lead}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads [post]
func (h *LeadHandler) Create(c *gin.Context) {
	var req CreateLeadRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if req.Status != "" {
		req.Status = lead.LeadStatus(strings.ToUpper(string(req.Status)))
	}

	// Try to get tenant ID from context
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	l, err := lead.NewLeadWithOptionalTenant(req.Name, req.Email, req.Phone, req.Avatar, req.Source, req.Status, req.PropertyInterest, req.AssignedBrokerID, tenantID)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	// Apply captação and client segmentation fields
	l.Role = req.Role
	l.IsClient = req.IsClient
	l.RecommendedPropertyID = strings.TrimSpace(req.RecommendedPropertyID)
	if req.RecommendedPropertySnapshot != nil {
		l.RecommendedPropertySnapshot = req.RecommendedPropertySnapshot
		if l.RecommendedPropertyID != "" && l.RecommendedPropertySnapshot.ID == "" {
			l.RecommendedPropertySnapshot.ID = l.RecommendedPropertyID
		}
	}
	l.CaptacaoPropertyID = req.CaptacaoPropertyID
	l.OwnerLeadID = req.OwnerLeadID
	l.PartnershipShared = req.PartnershipShared
	if req.Qualification != nil {
		l.Qualification = req.Qualification
	} else {
		l.Qualification = map[string]interface{}{}
	}

	if err := h.service.Create(c.Request.Context(), l); err != nil {
		// Mesmo mapeamento da atualização: entrada inválida (telefone
		// malformado, por exemplo) é 400 com o motivo; sem isso o cadastro de
		// lead respondia 500 genérico e o operador não sabia o que corrigir.
		respondLeadMutationError(c, err)
		return
	}

	if h.rouletteService != nil && strings.TrimSpace(l.AssignedBrokerID) == "" {
		_, _ = h.rouletteService.AssignByRoulette(c.Request.Context(), l, "")
	}

	c.Header("ETag", formatLeadETag(l))
	response.Created(c, "Lead created successfully", dto.MapLeadToDTO(l))
}

type UpdateLeadRequest struct {
	Name             *string                `json:"name"`
	Email            *string                `json:"email"`
	Phone            *string                `json:"phone"`
	Source           *lead.LeadSource       `json:"source"`
	Status           *lead.LeadStatus       `json:"status"`
	AssignedBrokerID *string                `json:"assigned_broker_id"`
	PropertyInterest *lead.LeadProperty     `json:"property_interest"`
	Role             *lead.ClientRole       `json:"role,omitempty"`
	Qualification    map[string]interface{} `json:"qualification,omitempty"`
	// Captação fields — updatable so the lead can be linked to a property after creation
	CaptacaoPropertyID *string `json:"captacao_property_id,omitempty"`
	OwnerLeadID        *string `json:"owner_lead_id,omitempty"`
	// PartnershipShared — per-lead opt-in to the partner shared-content feature.
	PartnershipShared *bool `json:"partnership_shared,omitempty"`
}

type ConvertToClientRequest struct {
	Role       lead.ClientRole `json:"role" binding:"required,oneof=owner buyer tenant guarantor"`
	ContractID string          `json:"contract_id,omitempty"`
}

// Update godoc
// @Summary      Update lead
// @Description  Update an existing lead
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Param        request body handlers.UpdateLeadRequest true "Update Lead Request"
// @Success      200  {object}  response.Response{data=lead.Lead}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/{id} [put]
func (h *LeadHandler) Update(c *gin.Context) {
	h.applyLeadPatch(c)
}

func (h *LeadHandler) Patch(c *gin.Context) {
	h.applyLeadPatch(c)
}

// RegisterFirstContact godoc
// @Summary      Registrar primeiro contato com o lead (#427)
// @Description  Marca FirstContactAt = now() e transita status para
// @Description  CONTACTED quando aplicável. Idempotente — chamadas
// @Description  subsequentes retornam o lead com o timestamp original.
// @Tags         leads
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Success      200 {object} response.Response{data=lead.Lead}
// @Failure      404 {object} response.Response
// @Router       /leads/{id}/first-contact [post]
func (h *LeadHandler) RegisterFirstContact(c *gin.Context) {
	leadID := c.Param("id")
	l, registered, err := h.service.RegisterFirstContact(c.Request.Context(), leadID)
	if err != nil {
		if err.Error() == "lead not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	msg := "Primeiro contato registrado"
	if !registered {
		msg = "Primeiro contato já havia sido registrado anteriormente"
	}
	response.Success(c, msg, l)
}

// DisqualifyLeadRequest — corpo de POST /leads/:id/disqualify (#429). O
// motivo é obrigatório (vai pro histórico do lead e pro audit trail).
type DisqualifyLeadRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// Disqualify godoc
// @Summary      Disqualify lead
// @Description  Marks a lead as DISQUALIFIED with a mandatory reason, recorded in the lead history and audit trail
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Param        request body handlers.DisqualifyLeadRequest true "Disqualify Lead Request"
// @Success      200  {object}  response.Response{data=lead.Lead}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/{id}/disqualify [post]
func (h *LeadHandler) Disqualify(c *gin.Context) {
	leadID := c.Param("id")

	var req DisqualifyLeadRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	actor, _ := pkgcontext.GetAccountID(c.Request.Context())
	l, err := h.service.Disqualify(c.Request.Context(), leadID, req.Reason, actor)
	if err != nil {
		if err.Error() == "lead not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Lead desqualificado", l)
}

// SharedLeads godoc
// @Summary      List all partner-shared leads
// @Description  Aggregates the leads shared with the caller across every ACTIVE partnership that grants the leads view scope. Each lead is privacy-projected (LGPD); the response carries the originating partnership id and source company id.
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]service.SharedLead}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships/shared/leads [get]
func (h *LeadHandler) SharedLeads(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	leads, err := h.service.SharedLeadsForCompany(c.Request.Context(), tenantID)
	if err != nil {
		if err.Error() == "access denied" {
			response.Forbidden(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Shared leads retrieved successfully", leads)
}

// formatLeadETag renders a weak ETag combining the monotonic updated_seq with
// the legacy nano-second timestamp. The seq prefix is what the repository
// actually enforces on PATCH; the timestamp is kept inside the token so
// pre-1.6 clients that round-trip the value still see a value that the
// parser accepts.
func formatLeadETag(l *lead.Lead) string {
	if l == nil {
		return ""
	}
	return fmt.Sprintf(`W/"%d:%s"`, l.UpdatedSeq, l.UpdatedAt.UTC().Format(time.RFC3339Nano))
}

// parseIfMatchLead extracts the optimistic-locking expectation from an
// If-Match header. It accepts three formats for backward compatibility:
//   - RFC3339Nano timestamp (pre-1.6 clients)
//   - W/"<seq>:<rfc3339nano>" (1.6+ clients)
//   - W/"<seq>"               (future minimal form)
//
// Returns nil when the header is empty or unparseable; callers should treat
// a nil result as "no concurrency check requested".
func parseIfMatchLead(v string) *lead.LeadExpectedVersion {
	s := strings.TrimSpace(v)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "W/") {
		s = strings.TrimSpace(strings.TrimPrefix(s, "W/"))
	}
	s = strings.Trim(s, "\"")
	if s == "" {
		return nil
	}
	out := &lead.LeadExpectedVersion{}
	// Try the legacy formats first (RFC3339Nano contains ':' which would
	// confuse the strings.Cut below).
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		out.UpdatedAt = &t
		return out
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		out.UpdatedAt = &t
		return out
	}
	// Combined `seq:rfc3339nano` form — split on the FIRST ':' only.
	if body, tail, hasTail := strings.Cut(s, ":"); hasTail {
		if seq, err := strconv.ParseInt(body, 10, 64); err == nil {
			out.UpdatedSeq = &seq
		}
		if t, err := time.Parse(time.RFC3339Nano, tail); err == nil {
			out.UpdatedAt = &t
		}
		if out.HasCheck() {
			return out
		}
		return nil
	}
	// Minimal seq-only form.
	if seq, err := strconv.ParseInt(s, 10, 64); err == nil {
		out.UpdatedSeq = &seq
		return out
	}
	return nil
}

func (h *LeadHandler) applyLeadPatch(c *gin.Context) {
	id := c.Param("id")
	var req UpdateLeadRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	l, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "Lead not found")
		return
	}

	updates := map[string]interface{}{}

	if req.Name != nil {
		updates["Name"] = *req.Name
	}
	if req.Email != nil {
		updates["Email"] = *req.Email
	}
	if req.Phone != nil {
		updates["Phone"] = *req.Phone
	}
	if req.Source != nil {
		updates["Source"] = *req.Source
	}
	if req.Status != nil && *req.Status != l.Status {
		oldStatus := l.Status
		l.Status = lead.LeadStatus(strings.ToUpper(string(*req.Status)))
		userID, _ := pkgcontext.GetAccountID(c.Request.Context())
		l.AddHistory(lead.LeadActionStatusChanged, userID, "", &oldStatus, &l.Status)
		updates["Status"] = l.Status
		updates["History"] = l.History
	}
	if req.AssignedBrokerID != nil {
		// SECURITY (achado A26): trocar o dono do lead é `leads:reassign`
		// (admin/gestor/operacoes). Sem isto, qualquer `leads:write` tomava o
		// lead de outro corretor pelo corpo do PATCH.
		if h.permissionProvider == nil || !auth.HasPermission(c, h.permissionProvider, "leads:reassign") {
			response.Forbidden(c, "permission denied: leads:reassign is required to change the assigned broker")
			return
		}
		updates["AssignedBrokerID"] = *req.AssignedBrokerID
	}
	if req.PropertyInterest != nil {
		updates["PropertyInterest"] = req.PropertyInterest
	}
	if req.Role != nil {
		updates["Role"] = *req.Role
	}
	if req.CaptacaoPropertyID != nil {
		updates["CaptacaoPropertyID"] = *req.CaptacaoPropertyID
	}
	if req.OwnerLeadID != nil {
		updates["OwnerLeadID"] = *req.OwnerLeadID
	}
	if req.Qualification != nil {
		updates["Qualification"] = req.Qualification
	}
	// PartnershipShared is persisted under the lowercase `partnership_shared`
	// JSONB key (the field carries an explicit json tag), so the jsonb_set
	// path must use that exact key — not the PascalCase Go field name.
	if req.PartnershipShared != nil {
		updates["partnership_shared"] = *req.PartnershipShared
	}

	expected := parseIfMatchLead(c.GetHeader("If-Match"))
	updated, ok, err := h.service.Patch(c.Request.Context(), id, tenantID, expected, updates)
	if err != nil {
		respondLeadMutationError(c, err)
		return
	}
	if !ok {
		if expected != nil {
			response.Conflict(c, "Lead foi atualizado por outra pessoa. Recarregue e tente novamente.")
			return
		}
		response.NotFound(c, "Lead not found")
		return
	}

	c.Header("ETag", formatLeadETag(updated))
	response.Success(c, "Lead updated successfully", dto.MapLeadToDTO(updated))
}

// respondLeadMutationError mapeia o erro de uma mutação de lead para a
// resposta HTTP: validação de entrada vira 400 com o motivo, o resto segue
// como 500. Centralizado porque criar e atualizar precisam da MESMA regra —
// quando só a atualização tinha o mapeamento, um telefone malformado no
// cadastro retornava "erro interno" e gerava ticket falso.
func respondLeadMutationError(c *gin.Context, err error) {
	if isValidationError(err) {
		response.ValidationError(c, err.Error())
		return
	}
	response.InternalError(c, err)
}

// isValidationError reports whether the error came from input validation
// (bad phone, missing field, etc.) and should be surfaced as 400 instead of
// 500. Hoje é heurístico via substring porque a maioria dos validators
// retorna `fmt.Errorf("invalid X: %w", ...)`; quando migrarmos para
// sentinel errors via errors.Is, este helper troca o predicate sem
// quebrar nada.
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	// A regra de nome de pessoa tem origem compartilhada com o resto do
	// produto (`pkg/validation`): reconhecer pelo helper evita divergir do
	// texto que o formulário mostra.
	if validation.IsPersonNameError(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.HasPrefix(msg, "invalid phone"),
		strings.HasPrefix(msg, "invalid cpf"),
		strings.HasPrefix(msg, "invalid cnpj"),
		strings.HasPrefix(msg, "invalid email"),
		strings.HasPrefix(msg, "invalid_"),
		strings.Contains(msg, "validation"):
		return true
	}
	return false
}

// List godoc
// @Summary      List leads
// @Description  List leads with pagination
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        limit query int false "Limit (default 25, max 100, tenant scope max 10)"
// @Param        offset query int false "Offset"
// @Param        status query string false "Status"
// @Param        search query string false "Search"
// @Param        role query string false "Role (owner, buyer, tenant, guarantor)"
// @Param        is_client query bool false "Is Client"
// @Success      200  {object}  response.Response{data=[]lead.Lead}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads [get]
func (h *LeadHandler) List(c *gin.Context) {
	// P0/A10 (reauditoria de 2026-09-20) — o escopo de visibilidade é decidido
	// pelo SERVIDOR; `?scope=` é preferência do cliente, que pode pedir MENOS
	// visão e nunca mais.
	//
	// Antes, pedir `?scope=tenant` simplesmente deixava de aplicar o filtro por
	// corretor (`brokerID` ficava vazio, e ele é o único predicado de corretor
	// da consulta). Como `leads:read` é default de corretor, marketing e
	// auditor (pkg/auth/permission_catalog.go), qualquer um deles paginava a
	// base inteira do tenant — com PII — só trocando a query string.
	requestedScope := c.DefaultQuery("scope", "broker")
	userProfile, _ := pkgcontext.GetUserPermission(c.Request.Context())
	functionalRole, _ := pkgcontext.GetFunctionalRole(c.Request.Context())
	baseRole, _ := pkgcontext.GetBaseRole(c.Request.Context())
	accountID, _ := pkgcontext.GetAccountID(c.Request.Context())

	// A regra de "quem vê o tenant inteiro" vive em pkg/auth e é a MESMA usada
	// pelo toca-deal (negócios, contratos, documentos). Antes cada serviço tinha
	// a sua — e elas discordavam (incidente de 2026-09-21).
	isElevated := pkgAuth.HasTenantWideScope(userProfile, functionalRole, baseRole)

	// Papel não elevado nunca alcança o tenant inteiro, mesmo pedindo: o pedido
	// é rebaixado para o escopo da própria carteira.
	scope := requestedScope
	if !isElevated {
		scope = "broker"
	}

	c.Header("X-Max-Limit", strconv.FormatInt(MaxLeadListLimit, 10))
	limit, errMsg := ParseLeadListLimit(scope, c.Query("limit"))
	if errMsg != "" {
		response.ValidationError(c, errMsg)
		return
	}

	// Handle pagination: use offset if provided, otherwise calculate from page
	var offset int64
	if offsetStr := c.Query("offset"); offsetStr != "" {
		offset, _ = strconv.ParseInt(offsetStr, 10, 64)
	} else {
		page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 64)
		if page < 1 {
			page = 1
		}
		offset = (page - 1) * limit
	}

	status := c.Query("status")
	search := c.Query("search")
	leadRole := c.Query("role")
	if scope == "tenant" && len(strings.TrimSpace(search)) < 2 && c.Query("email") == "" && c.Query("phone") == "" && c.Query("display_id") == "" {
		response.ValidationError(c, "search required for tenant scope")
		return
	}

	var isClient *bool
	if isClientStr := c.Query("is_client"); isClientStr != "" {
		val, err := strconv.ParseBool(isClientStr)
		if err != nil {
			response.ValidationError(c, "invalid is_client value")
			return
		}
		isClient = &val
	}

	// Try to get tenant ID from context
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	// O recorte por corretor vem do papel resolvido no topo de List — não do
	// `?scope=` pedido. Não elevado é sempre a própria carteira.
	var brokerID string
	if !isElevated {
		brokerID = accountID
	}

	parseCSV := func(v string) []string {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil
		}
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		seen := map[string]struct{}{}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			p = strings.ToUpper(p)
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
		if len(out) == 0 {
			return nil
		}
		return out
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

	parseIntPtr := func(v string) (*int, error) {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, nil
		}
		i, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}
		return &i, nil
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

	parseBoolPtr := func(v string) (*bool, error) {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, nil
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, err
		}
		return &b, nil
	}

	assignedFrom, err := parseTime(c.Query("assigned_from"))
	if err != nil {
		response.ValidationError(c, "invalid assigned_from value")
		return
	}
	assignedTo, err := parseTime(c.Query("assigned_to"))
	if err != nil {
		response.ValidationError(c, "invalid assigned_to value")
		return
	}
	expiresBefore, err := parseTime(c.Query("expires_before"))
	if err != nil {
		response.ValidationError(c, "invalid expires_before value")
		return
	}
	expiresAfter, err := parseTime(c.Query("expires_after"))
	if err != nil {
		response.ValidationError(c, "invalid expires_after value")
		return
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
	updatedFrom, err := parseTime(c.Query("updated_from"))
	if err != nil {
		response.ValidationError(c, "invalid updated_from value")
		return
	}
	updatedTo, err := parseTime(c.Query("updated_to"))
	if err != nil {
		response.ValidationError(c, "invalid updated_to value")
		return
	}
	lastInteractionFrom, err := parseTime(c.Query("last_interaction_from"))
	if err != nil {
		response.ValidationError(c, "invalid last_interaction_from value")
		return
	}
	lastInteractionTo, err := parseTime(c.Query("last_interaction_to"))
	if err != nil {
		response.ValidationError(c, "invalid last_interaction_to value")
		return
	}

	escalationLevelMin, err := parseIntPtr(c.Query("escalation_level_min"))
	if err != nil {
		response.ValidationError(c, "invalid escalation_level_min value")
		return
	}
	rouletteRetriesMin, err := parseIntPtr(c.Query("roulette_retries_min"))
	if err != nil {
		response.ValidationError(c, "invalid roulette_retries_min value")
		return
	}
	staleDays, err := parseIntPtr(c.Query("stale_days"))
	if err != nil {
		response.ValidationError(c, "invalid stale_days value")
		return
	}

	interestMinPrice, err := parseFloatPtr(c.Query("interest_min_price"))
	if err != nil {
		response.ValidationError(c, "invalid interest_min_price value")
		return
	}
	interestMaxPrice, err := parseFloatPtr(c.Query("interest_max_price"))
	if err != nil {
		response.ValidationError(c, "invalid interest_max_price value")
		return
	}
	interestBedroomsMin, err := parseIntPtr(c.Query("interest_bedrooms_min"))
	if err != nil {
		response.ValidationError(c, "invalid interest_bedrooms_min value")
		return
	}
	interestBathroomsMin, err := parseIntPtr(c.Query("interest_bathrooms_min"))
	if err != nil {
		response.ValidationError(c, "invalid interest_bathrooms_min value")
		return
	}
	interestParkingMin, err := parseIntPtr(c.Query("interest_parking_min"))
	if err != nil {
		response.ValidationError(c, "invalid interest_parking_min value")
		return
	}
	interestAreaMin, err := parseFloatPtr(c.Query("interest_area_min"))
	if err != nil {
		response.ValidationError(c, "invalid interest_area_min value")
		return
	}
	interestAreaMax, err := parseFloatPtr(c.Query("interest_area_max"))
	if err != nil {
		response.ValidationError(c, "invalid interest_area_max value")
		return
	}

	qualified, err := parseBoolPtr(c.Query("qualified"))
	if err != nil {
		response.ValidationError(c, "invalid qualified value")
		return
	}
	unassigned, err := parseBoolPtr(c.Query("unassigned"))
	if err != nil {
		response.ValidationError(c, "invalid unassigned value")
		return
	}
	isCaptacao, err := parseBoolPtr(c.Query("is_captacao"))
	if err != nil {
		response.ValidationError(c, "invalid is_captacao value")
		return
	}

	filter := lead.LeadFilter{
		Limit:  limit,
		Offset: offset,

		Status:   status,
		StatusIn: parseCSV(c.Query("status_in")),
		Search:   search,

		DisplayID:  c.Query("display_id"),
		Email:      c.Query("email"),
		Phone:      c.Query("phone"),
		AccountID:  c.Query("account_id"),
		ContractID: c.Query("contract_id"),

		Source:   c.Query("source"),
		SourceIn: parseCSV(c.Query("source_in")),

		Tag:       c.Query("tag"),
		Qualified: qualified,

		Role:     leadRole,
		IsClient: isClient,

		AssignedBrokerID: c.Query("assigned_broker_id"),
		Unassigned:       unassigned,
		BrokerScopeID:    brokerID,

		AssignedFrom: assignedFrom,
		AssignedTo:   assignedTo,

		ExpiresBefore: expiresBefore,
		ExpiresAfter:  expiresAfter,

		EscalationLevelMin: escalationLevelMin,
		RouletteRetriesMin: rouletteRetriesMin,
		TierID:             c.Query("tier_id"),

		CreatedFrom: createdFrom,
		CreatedTo:   createdTo,
		UpdatedFrom: updatedFrom,
		UpdatedTo:   updatedTo,

		LastInteractionFrom: lastInteractionFrom,
		LastInteractionTo:   lastInteractionTo,
		StaleDays:           staleDays,

		PropertyID:         c.Query("property_id"),
		CaptacaoPropertyID: c.Query("captacao_property_id"),
		OwnerLeadID:        c.Query("owner_lead_id"),
		IsCaptacao:         isCaptacao,

		InterestType:         c.Query("interest_type"),
		InterestLocation:     c.Query("interest_location"),
		InterestMinPrice:     interestMinPrice,
		InterestMaxPrice:     interestMaxPrice,
		InterestBedroomsMin:  interestBedroomsMin,
		InterestBathroomsMin: interestBathroomsMin,
		InterestParkingMin:   interestParkingMin,
		InterestAreaMin:      interestAreaMin,
		InterestAreaMax:      interestAreaMax,

		NoteSearch: c.Query("note_search"),
	}

	leads, total, err := h.service.List(c.Request.Context(), tenantID, filter)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Leads retrieved successfully", []dto.LeadResponse{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Leads retrieved successfully", dto.MapLeadsToDTO(leads), page, int(limit), total)
}

func (h *LeadHandler) GetByEmail(c *gin.Context) {
	email := c.Param("email")

	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	l, err := h.service.GetByEmail(c.Request.Context(), tenantID, email)
	if err != nil || l == nil {
		response.NotFound(c, "Lead not found")
		return
	}

	c.Header("ETag", formatLeadETag(l))
	response.Success(c, "Lead retrieved successfully", dto.MapLeadToDTO(l))
}

func (h *LeadHandler) GetByPhone(c *gin.Context) {
	phone := c.Param("phone")

	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	l, err := h.service.GetByPhone(c.Request.Context(), tenantID, phone)
	if err != nil || l == nil {
		response.NotFound(c, "Lead not found")
		return
	}

	c.Header("ETag", formatLeadETag(l))
	response.Success(c, "Lead retrieved successfully", dto.MapLeadToDTO(l))
}

func (h *LeadHandler) GetByDisplayID(c *gin.Context) {
	displayID := c.Param("display_id")

	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	l, err := h.service.GetByDisplayID(c.Request.Context(), tenantID, displayID)
	if err != nil || l == nil {
		response.NotFound(c, "Lead not found")
		return
	}

	c.Header("ETag", formatLeadETag(l))
	response.Success(c, "Lead retrieved successfully", dto.MapLeadToDTO(l))
}

// GetByID godoc
// @Summary      Get lead by ID
// @Description  Get a lead by its ID
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Success      200  {object}  response.Response{data=lead.Lead}
// @Failure      404  {object}  response.Response
// @Router       /leads/{id} [get]
// canReadLead decide se o chamador pode ler ESTE lead.
//
// SECURITY (achado A10 da revisão de permissões de 2026-09-19): a rota usava
// `RequirePermission(provider, "")` e não checava posse — qualquer papel lia
// qualquer lead do tenant pelo UUID. A UI já aplicava a regra de visibilidade
// por corretor no cliente (`isAdmin` em Leads.tsx); agora o servidor aplica a
// MESMA regra: quem pode reatribuir lead (admin/gestor/operacoes) lê todos;
// os demais leem o que está atribuído a eles ou ainda não atribuído (o pool da
// roleta, que a tela também mostra).
// canMutateLead decide se o chamador pode ALTERAR este lead.
//
// SECURITY (achado A26 da revisão de permissões de 2026-09-19): as rotas de
// mutação (`PUT/PATCH /leads/:id`, notas, desqualificar, converter, primeiro
// contato) exigiam apenas `leads:write` e não checavam posse — um corretor
// alterava o lead de outro corretor do mesmo tenant. A regra é a mesma da
// leitura, já que a listagem do corretor inclui o pool NÃO atribuído (o
// repositório filtra `AssignedBrokerID = caller OR AssignedBrokerID = ”`):
// quem pode reatribuir altera qualquer lead; os demais alteram o próprio ou o
// ainda não atribuído.
func canMutateLead(c *gin.Context, l *lead.Lead, provider auth.PermissionProvider) bool {
	return canReadLead(c, l, provider)
}

// RequireLeadOwnership valida, ANTES do handler, que o chamador pode agir sobre
// o lead do path. Aplicado às rotas de mutação para que a checagem exista em um
// só lugar (e não dependa de cada handler lembrar de fazê-la).
//
// Responde 404 quando o lead não existe OU não pertence ao escopo do chamador —
// não confirma a existência de lead alheio.
func (h *LeadHandler) RequireLeadOwnership() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.Next()
			return
		}
		l, err := h.service.GetByID(c.Request.Context(), id)
		if err != nil || l == nil {
			response.NotFound(c, "lead not found")
			c.Abort()
			return
		}
		if !canMutateLead(c, l, h.permissionProvider) {
			response.NotFound(c, "lead not found")
			c.Abort()
			return
		}
		// Disponibiliza o agregado para o handler não precisar reler.
		c.Set("lead_autorizado", l)
		c.Next()
	}
}

func canReadLead(c *gin.Context, l *lead.Lead, provider auth.PermissionProvider) bool {
	if l == nil {
		return false
	}
	if provider != nil && auth.HasPermission(c, provider, "leads:reassign") {
		return true
	}
	accountID, _ := pkgcontext.GetAccountID(c.Request.Context())
	if accountID == "" {
		return false
	}
	if l.AssignedBrokerID == "" {
		return true
	}
	return l.AssignedBrokerID == accountID
}

// respondTenantContextError (B10) devolve 403 quando o serviço falhou por
// ausência de tenant no contexto e informa se a resposta já foi escrita. Sem
// isso o chamador sem tenant recebia 404 ("lead not found"), indistinguível de
// recurso inexistente.
func respondTenantContextError(c *gin.Context, err error) bool {
	if errors.Is(err, service.ErrTenantContextRequired) {
		response.Forbidden(c, "tenant context required")
		return true
	}
	return false
}

func (h *LeadHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	l, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if respondTenantContextError(c, err) {
			return
		}
		response.NotFound(c, err.Error())
		return
	}
	if !canReadLead(c, l, h.permissionProvider) {
		// 404 (e não 403) de propósito: não confirma a existência do lead alheio.
		response.NotFound(c, "lead not found")
		return
	}
	c.Header("ETag", formatLeadETag(l))
	response.Success(c, "Lead retrieved successfully", dto.MapLeadToDTO(l))
}

// GetByIDInternal is the service-to-service equivalent of GetByID. Takes
// tenant_id explicitly via query so the call doesn't depend on a user JWT.
// Sits behind X-Internal-Key auth in the routes layer.
func (h *LeadHandler) GetByIDInternal(c *gin.Context) {
	id := c.Param("id")

	// SECURITY (achado A3): antes o `tenant_id` era lido do query, validado como
	// não-vazio e depois IGNORADO (o serviço resolvia o tenant do contexto, que
	// numa rota interna não existe). Agora o tenant vem da credencial resolvida:
	// JWT de usuário ⇒ tenant do token; chave de serviço ⇒ tenant explícito,
	// restrito à allowlist quando configurada.
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

	l, err := h.service.GetByIDForTenant(c.Request.Context(), tenantID, id)
	if err != nil || l == nil {
		response.NotFound(c, "lead not found")
		return
	}
	response.Success(c, "Lead retrieved successfully", dto.MapLeadToDTO(l))
}

// GetStats godoc
// @Summary      Get lead stats
// @Description  Get lead stats
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Router       /leads/stats [get]
func (h *LeadHandler) GetStats(c *gin.Context) {
	// Try to get tenant ID from context
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	userProfile, _ := pkgcontext.GetUserPermission(c.Request.Context())
	functionalRole, _ := pkgcontext.GetFunctionalRole(c.Request.Context())
	baseRole, _ := pkgcontext.GetBaseRole(c.Request.Context())
	accountID, _ := pkgcontext.GetAccountID(c.Request.Context())

	var brokerID string
	if !pkgAuth.HasTenantWideScope(userProfile, functionalRole, baseRole) {
		brokerID = accountID
	}

	total, byStatus, conversionRate, err := h.service.GetStats(c.Request.Context(), tenantID, brokerID) // fetch stats
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Stats retrieved successfully", gin.H{
				"total":           0,
				"by_status":       map[string]int64{},
				"conversion_rate": 0,
			})
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Stats retrieved successfully", gin.H{
		"total":           total,
		"by_status":       byStatus,
		"conversion_rate": conversionRate,
	})
}

// GetRouletteConfig godoc
// @Summary      Get roulette configuration
// @Description  Get roulette configuration for the tenant
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=lead.RouletteConfig}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/roulette [get]
func (h *LeadHandler) GetRouletteConfig(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	config, err := h.rouletteService.GetRoulette(c.Request.Context(), tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	// Wave P1.4 — surfaces "silent misconfiguration" cases (weighted sem
	// pesos, priority sem flag etc) como warnings opacos para o FE
	// exibir como banner. Custa uma chamada extra ao broker repo só no
	// GET; o admin não fica adivinhando por que a roleta age "estranho".
	warnings := h.rouletteService.ConfigWarnings(c.Request.Context(), tenantID, config)
	resp := dto.MapRouletteConfigToDTO(config)
	if resp != nil {
		resp.Warnings = warnings
	}

	response.Success(c, "Roulette config retrieved successfully", resp)
}

// GetRouletteUnassignedStatus godoc
// @Summary      Check unassigned leads when enabling roulette
// @Description  Returns whether there are leads without assigned broker for the tenant
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/roulette/unassigned [get]
func (h *LeadHandler) GetRouletteUnassignedStatus(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	count, err := h.rouletteService.CountUnassignedLeads(c.Request.Context(), tenantID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Unassigned leads status retrieved successfully", gin.H{"has_unassigned": false, "count": int64(0)})
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Unassigned leads status retrieved successfully", gin.H{"has_unassigned": count > 0, "count": count})
}

// UpdateRouletteConfig godoc
// @Summary      Update roulette configuration
// @Description  Update roulette configuration for the tenant
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body lead.UpdateRouletteConfigDTO true "Update Roulette Config Request"
// @Success      200  {object}  response.Response{data=lead.RouletteConfig}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/roulette [put]
func (h *LeadHandler) UpdateRouletteConfig(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	var req lead.UpdateRouletteConfigDTO
	if !validation.BindJSON(c, &req) {
		return
	}

	actorID, _ := pkgcontext.GetAccountID(c.Request.Context())
	actorEmail, _ := pkgcontext.GetEmail(c.Request.Context())
	config, err := h.rouletteService.UpdateConfig(c.Request.Context(), tenantID, req, service.RouletteActor{ID: actorID, Email: actorEmail})
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Roulette config updated successfully", dto.MapRouletteConfigToDTO(config))
}

type SimulateRouletteRequest struct {
	Count   int    `json:"count" binding:"required,min=1"`
	Stage   string `json:"stage,omitempty"`
	Persist bool   `json:"persist,omitempty"`
	Seed    int64  `json:"seed,omitempty"`
}

// SimulateRoulette godoc
// @Summary      Simulate roulette distribution
// @Description  Simulate round-robin distribution for N leads without creating leads. If persist=true, advances the roulette pointer.
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.SimulateRouletteRequest true "Simulation Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/roulette/simulate [post]
func (h *LeadHandler) SimulateRoulette(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	var req SimulateRouletteRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	res, err := h.rouletteService.SimulateRoulette(c.Request.Context(), tenantID, req.Stage, req.Count, req.Persist, req.Seed)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Simulation completed", res)
}

type AddNoteRequest struct {
	Content string `json:"content" binding:"required"`
}

// AddNote godoc
// @Summary      Add note to lead
// @Description  Add a manual note to a lead
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Param        request body handlers.AddNoteRequest true "Add Note Request"
// @Success      200  {object}  response.Response{data=lead.Lead}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/{id}/notes [post]
func (h *LeadHandler) AddNote(c *gin.Context) {
	id := c.Param("id")
	var req AddNoteRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	l, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "Lead not found")
		return
	}

	userID, _ := pkgcontext.GetAccountID(c.Request.Context())
	l.AddHistory(lead.LeadActionNote, userID, req.Content, nil, nil)

	if err := h.service.Update(c.Request.Context(), l); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Note added successfully", dto.MapLeadToDTO(l))
}

// InviteClient godoc
// @Summary      Invite lead to portal
// @Description  Sends an invitation to the lead to access the client portal
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/{id}/invite [post]
func (h *LeadHandler) InviteClient(c *gin.Context) {
	id := c.Param("id")

	if err := h.service.InviteClient(c.Request.Context(), id); err != nil {
		if err.Error() == "lead not found" {
			response.NotFound(c, err.Error())
		} else if err.Error() == "lead must have an email to be invited" {
			response.Error(c, 400, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Client invited successfully", nil)
}

// @Summary      Convert lead to client
// @Description  Promote a lead to a client with a specified role (owner, buyer, tenant, guarantor), preserving the unique ID
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Param        request body handlers.ConvertToClientRequest true "Convert to Client Request"
// @Success      200  {object}  response.Response{data=lead.Lead}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /leads/{id}/convert [post]
func (h *LeadHandler) ConvertToClient(c *gin.Context) {
	id := c.Param("id")
	var req ConvertToClientRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	l, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "Lead not found")
		return
	}

	l.ConvertToClient(req.Role, req.ContractID)

	userID, _ := pkgcontext.GetAccountID(c.Request.Context())
	converted := lead.LeadStatusConverted
	l.AddHistory(lead.LeadActionStatusChanged, userID, "Lead converted to client with role: "+string(req.Role), nil, &converted)

	if err := h.service.Update(c.Request.Context(), l); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Lead converted to client successfully", dto.MapLeadToDTO(l))
}

// RejectAssignmentRequest é o body de POST /leads/:id/reject-assignment.
// Reason é obrigatório — usado pelo admin para auditar comportamento da
// roleta (sem capacity? especialidade errada? conflito de interesse?).
type RejectAssignmentRequest struct {
	Reason string `json:"reason" binding:"required,min=2,max=500"`
}

// AcceptAssignment godoc
//
// @Summary      Accept lead assignment (broker accepts roulette lead)
// @Description  Broker accepts a lead assigned by the roulette. Lead must be in PENDING_ACCEPTANCE; broker_id from JWT must match assigned_broker_id. Lead transitions to CONTACTED.
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Success      200  {object}  response.Response{data=lead.Lead}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response  "lead is not assigned to caller"
// @Failure      404  {object}  response.Response  "lead not found"
// @Failure      409  {object}  response.Response  "lead status does not allow acceptance"
// @Failure      500  {object}  response.Response
// @Router       /leads/{id}/accept-assignment [post]
func (h *LeadHandler) AcceptAssignment(c *gin.Context) {
	id := c.Param("id")
	// O broker_id vem do JWT (account_id do corretor logado) — não
	// aceitamos via body para evitar spoofing entre corretores.
	brokerID, _ := pkgcontext.GetAccountID(c.Request.Context())
	if brokerID == "" {
		response.Forbidden(c, "broker identity required")
		return
	}

	l, err := h.service.AcceptAssignment(c.Request.Context(), id, brokerID)
	if err != nil {
		msg := err.Error()
		switch {
		case msg == "lead not found":
			response.NotFound(c, msg)
		case msg == "lead is not assigned" ||
			msg == "lead is not assigned to caller":
			response.Forbidden(c, msg)
		case strings.HasPrefix(msg, "lead status"):
			response.Conflict(c, msg)
		default:
			response.InternalServerError(c, msg)
		}
		return
	}

	response.Success(c, "Lead acceptance recorded", dto.MapLeadToDTO(l))
}

// RejectAssignment godoc
//
// @Summary      Reject lead assignment (broker declines roulette lead)
// @Description  Broker rejects a lead assigned by the roulette. Reason is required. Lead returns to NEW status and re-enters roulette on next sweep.
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Param        request body handlers.RejectAssignmentRequest true "Reject reason"
// @Success      200  {object}  response.Response{data=lead.Lead}
// @Failure      400  {object}  response.Response  "reason is required"
// @Failure      403  {object}  response.Response  "lead is not assigned to caller"
// @Failure      404  {object}  response.Response  "lead not found"
// @Failure      409  {object}  response.Response  "lead status does not allow rejection"
// @Failure      500  {object}  response.Response
// @Router       /leads/{id}/reject-assignment [post]
func (h *LeadHandler) RejectAssignment(c *gin.Context) {
	id := c.Param("id")
	brokerID, _ := pkgcontext.GetAccountID(c.Request.Context())
	if brokerID == "" {
		response.Forbidden(c, "broker identity required")
		return
	}

	var req RejectAssignmentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	l, err := h.service.RejectAssignment(c.Request.Context(), id, brokerID, req.Reason)
	if err != nil {
		msg := err.Error()
		switch {
		case msg == "lead not found":
			response.NotFound(c, msg)
		case msg == "lead is not assigned" ||
			msg == "lead is not assigned to caller":
			response.Forbidden(c, msg)
		case strings.HasPrefix(msg, "lead status"):
			response.Conflict(c, msg)
		case msg == "reason is required for reject":
			response.Error(c, 400, msg)
		default:
			response.InternalServerError(c, msg)
		}
		return
	}

	response.Success(c, "Lead rejection recorded", dto.MapLeadToDTO(l))
}

// GetMatchedProperties godoc
//
// @Summary      Match Properties (cruzamento inteligente)
// @Description  Devolve imóveis disponíveis que batem com as preferências do lead, rankeados por score (faixa de preço, localização, área, quartos). Issue #415.
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Lead ID"
// @Param        limit query int false "Max matches (default 10, cap 100)"
// @Success      200  {object}  response.Response{data=[]service.MatchedProperty}
// @Failure      403  {object}  response.Response  "tenant context required"
// @Failure      404  {object}  response.Response  "lead not found"
// @Failure      422  {object}  response.Response  "lead has no preferences"
// @Failure      500  {object}  response.Response
// @Router       /leads/{id}/matched-properties [get]
func (h *LeadHandler) GetMatchedProperties(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	limit := 10
	if v := c.Query("limit"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil {
			limit = parsed
		}
	}

	matches, err := h.service.MatchProperties(c.Request.Context(), tenantID, id, limit)
	if err != nil {
		msg := err.Error()
		switch {
		case errors.Is(err, service.ErrLeadHasNoPreferences):
			// 422: dado precisa ser corrigido antes do match fazer sentido.
			response.UnprocessableEntity(c, msg)
			return
		case msg == "lead not found":
			response.NotFound(c, msg)
			return
		}
		response.InternalServerError(c, msg)
		return
	}

	response.Success(c, "Matched properties retrieved", matches)
}

// GetClientPortalOverview godoc
// @Summary      Client portal overview (lead + imóveis recomendados)
// @Description  Resolve o(s) lead(s) do cliente convidado (account_id) e,
// @Description  quando houver preferências, os imóveis recomendados (match #415).
// @Tags         portal
// @Accept       json
// @Produce      json
// @Success      200  {object}  response.Response{data=[]service.ClientPortalOverviewLead}
// @Router       /client-portal/overview [get]
func (h *LeadHandler) GetClientPortalOverview(c *gin.Context) {
	accountID, err := pkgcontext.GetAccountID(c.Request.Context())
	if err != nil || accountID == "" {
		response.Forbidden(c, "user context required")
		return
	}
	email, _ := pkgcontext.GetEmail(c.Request.Context())
	leads, err := h.service.ClientPortalOverview(c.Request.Context(), accountID, email)
	if err != nil {
		respondVisitError(c, err)
		return
	}
	response.Success(c, "Client portal overview retrieved", leads)
}

// GetClientPortalProperties godoc
// @Summary      Vitrine do cliente (sem negócio aberto)
// @Description  Imóveis disponíveis das imobiliárias com quem o cliente se relaciona (tenants dos leads dele). Mesmo recorte de disponibilidade e finalidade da busca do negócio.
// @Tags         client-portal
// @Produce      json
// @Security     BearerAuth
// @Param        listing_type query string false "sale|rent"
// @Param        limit query int false "Limit"
// @Success      200 {object} response.Response
// @Router       /client-portal/properties [get]
func (h *LeadHandler) GetClientPortalProperties(c *gin.Context) {
	accountID, err := pkgcontext.GetAccountID(c.Request.Context())
	if err != nil || accountID == "" {
		response.Forbidden(c, "user context required")
		return
	}
	email, _ := pkgcontext.GetEmail(c.Request.Context())
	listingType := c.Query("listing_type")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))

	properties, err := h.service.ClientPortalPortfolio(c.Request.Context(), accountID, email, listingType, limit)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Client portal properties retrieved", properties)
}

// SetRecommendedProperty godoc
// @Router       /leads/{id}/recommended-property [put]
// UpdateLeadVisit godoc @Router /client-portal/leads/{id}/visit [put]
// OpenDirectChannel godoc
// @Router       /client-portal/leads/{id}/channel [post]
func (h *LeadHandler) OpenDirectChannel(c *gin.Context) {
	id := c.Param("id")
	channelID, err := h.service.EnsureDirectChannel(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrVisitForbidden) {
			response.Forbidden(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrVisitNotFound) {
			response.NotFound(c, "lead não encontrado")
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Canal aberto", gin.H{"channel_id": channelID})
}

func (h *LeadHandler) UpdateLeadVisit(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Date string `json:"date" binding:"required"`
		Time string `json:"time" binding:"required"`
		service.VisitDetails
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.service.UpdateLeadVisit(c.Request.Context(), id, req.Date, req.Time, req.VisitDetails); err != nil {
		respondVisitError(c, err)
		return
	}
	response.Success(c, "Visita atualizada", nil)
}

// CancelLeadVisit godoc @Router /client-portal/leads/{id}/visit [delete]
func (h *LeadHandler) CancelLeadVisit(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.CancelLeadVisit(c.Request.Context(), id); err != nil {
		respondVisitError(c, err)
		return
	}
	response.Success(c, "Visita cancelada", nil)
}

// ConfirmLeadVisit godoc @Router /leads/{id}/visit/confirm [put]
func (h *LeadHandler) ConfirmLeadVisit(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.ConfirmLeadVisit(c.Request.Context(), id); err != nil {
		respondVisitError(c, err)
		return
	}
	response.Success(c, "Visita confirmada", nil)
}

func (h *LeadHandler) SetRecommendedProperty(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		PropertyID string `json:"property_id" binding:"required"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.service.SetRecommendedProperty(c.Request.Context(), id, req.PropertyID); err != nil {
		switch {
		case err.Error() == "lead not found":
			response.NotFound(c, "lead não encontrado")
		case errors.Is(err, service.ErrRecommendedPropertyUnreadable):
			// Imóvel inexistente, excluído ou de outra imobiliária: a mensagem
			// diz o que fazer em vez de "erro interno".
			response.ValidationError(c, "imóvel não encontrado na sua carteira")
		default:
			response.InternalError(c, err)
		}
		return
	}
	response.Success(c, "Imóvel recomendado com sucesso", nil)
}

// RequestLeadVisit godoc
// @Router       /client-portal/leads/{id}/visit-request [post]
func (h *LeadHandler) RequestLeadVisit(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		PropertyID string `json:"property_id"`
		Date       string `json:"date" binding:"required"`
		Time       string `json:"time" binding:"required"`
		service.VisitDetails
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.service.RequestLeadVisit(c.Request.Context(), id, req.PropertyID, req.Date, req.Time, req.VisitDetails); err != nil {
		respondVisitError(c, err)
		return
	}
	response.Success(c, "Visita solicitada com sucesso", nil)
}

// DownloadLeadVisitCalendar entrega o .ics da visita marcada do próprio
// cliente. Pedido ainda pendente não vira evento no calendário dele.
func (h *LeadHandler) DownloadLeadVisitCalendar(c *gin.Context) {
	file, name, err := h.service.ClientVisitCalendar(c.Request.Context(), c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrVisitNotMarked):
			response.Error(c, http.StatusConflict, "A visita ainda não está marcada. Assim que o corretor confirmar, o arquivo fica disponível.")
		default:
			respondVisitError(c, err)
		}
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", file)
}

func respondVisitError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrVisitForbidden):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrVisitNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrVisitInvalid), errors.Is(err, clients.ErrCalendarInvalid):
		response.ValidationError(c, err.Error())
	case errors.Is(err, service.ErrVisitClosed), errors.Is(err, service.ErrVisitConflict), errors.Is(err, clients.ErrCalendarConflict), errors.Is(err, clients.ErrCalendarChanged):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, clients.ErrCalendarUnavailable):
		response.Error(c, http.StatusServiceUnavailable, clients.ErrCalendarUnavailable.Error())
	default:
		logger.Error("Failed to save or synchronize portal visit", "path", c.Request.URL.Path, "error", err.Error())
		response.InternalServerError(c, "Não foi possível salvar a visita. Tente novamente.")
	}
}

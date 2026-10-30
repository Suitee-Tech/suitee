package handlers

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-crm/internal/api/v1/dto"
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
)

type BrokerHandler struct {
	repo     broker.Repository
	eventBus events.EventBus
}

func NewBrokerHandler(repo broker.Repository) *BrokerHandler {
	return &BrokerHandler{repo: repo}
}

// SetEventBus opts the handler into emitting broker.updated for downstream
// projections (Deal.BrokerParticipations labels, analytics). Nil bus is a
// no-op so existing tests/callers don't need to wire a fake.
func (h *BrokerHandler) SetEventBus(bus events.EventBus) {
	h.eventBus = bus
}

type CreateBrokerRequest struct {
	ID             string                `json:"id,omitempty"`
	Name           string                `json:"name" binding:"required"`
	Role           string                `json:"role" binding:"required"`
	Email          string                `json:"email,omitempty"`
	Avatar         string                `json:"avatar,omitempty"`
	Active         *bool                 `json:"active,omitempty"`
	MaxActiveLeads *int                  `json:"max_active_leads,omitempty"`
	Specialties    []string              `json:"specialties,omitempty"`
	Regions        []string              `json:"regions,omitempty"`
	Weight         *float64              `json:"weight,omitempty"`
	IsPriority     *bool                 `json:"is_priority,omitempty"`
	Paused         *bool                 `json:"paused,omitempty"`
	BusinessHours  *broker.BusinessHours `json:"business_hours,omitempty"`
}

type UpdateBrokerRequest struct {
	Name           *string               `json:"name,omitempty"`
	Role           *string               `json:"role,omitempty"`
	Email          *string               `json:"email,omitempty"`
	Avatar         *string               `json:"avatar,omitempty"`
	Active         *bool                 `json:"active,omitempty"`
	MaxActiveLeads *int                  `json:"max_active_leads,omitempty"`
	Specialties    []string              `json:"specialties,omitempty"`
	Regions        []string              `json:"regions,omitempty"`
	Weight         *float64              `json:"weight,omitempty"`
	IsPriority     *bool                 `json:"is_priority,omitempty"`
	Paused         *bool                 `json:"paused,omitempty"`
	BusinessHours  *broker.BusinessHours `json:"business_hours,omitempty"`
}

type UpdateMyQualificationRequest struct {
	Specialties   []string              `json:"specialties,omitempty"`
	Regions       []string              `json:"regions,omitempty"`
	BusinessHours *broker.BusinessHours `json:"business_hours,omitempty"`
}

func (h *BrokerHandler) Create(c *gin.Context) {
	var req CreateBrokerRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	b := broker.New(req.Name, req.Role, req.Email, tenantID)
	if req.ID != "" {
		b.ID = req.ID
	}
	b.Avatar = req.Avatar
	if req.Active != nil {
		b.Active = *req.Active
	}
	if req.MaxActiveLeads != nil {
		b.MaxActiveLeads = *req.MaxActiveLeads
	}
	if req.Specialties != nil {
		b.Specialties = req.Specialties
	}
	if req.Regions != nil {
		b.Regions = req.Regions
	}
	if req.Weight != nil {
		b.Weight = *req.Weight
	}
	if req.IsPriority != nil {
		b.IsPriority = *req.IsPriority
	}
	if req.Paused != nil {
		b.Paused = *req.Paused
	}
	if req.BusinessHours != nil {
		b.BusinessHours = req.BusinessHours
	}

	if err := h.repo.Create(c.Request.Context(), b); err != nil {
		response.InternalError(c, err)
		return
	}

	// Notifica downstream (toca-comm → WebSocket) que um corretor foi criado, para que
	// clientes refaçam fetch sem esperar revalidate on focus.
	h.publishBrokerChanged(c, b)

	response.Created(c, "Broker created successfully", dto.MapBrokerToDTO(b))
}

func (h *BrokerHandler) List(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	limit, err := strconv.ParseInt(c.DefaultQuery("limit", "10"), 10, 64)
	if err != nil || limit <= 0 {
		limit = 10
	}
	offset, err := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	if err != nil || offset < 0 {
		offset = 0
	}

	var active *bool
	if v := strings.TrimSpace(c.Query("active")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			active = &b
		}
	}
	var paused *bool
	if v := strings.TrimSpace(c.Query("paused")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			paused = &b
		}
	}
	var isPriority *bool
	if v := strings.TrimSpace(c.Query("is_priority")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			isPriority = &b
		}
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

	filter := broker.BrokerFilter{
		Search:      c.Query("search"),
		Specialties: parseCSV(c.Query("specialty")),
		Regions:     parseCSV(c.Query("region")),
		Active:      active,
		Paused:      paused,
		Role:        c.Query("role"),
		IsPriority:  isPriority,
		Limit:       limit,
		Offset:      offset,
	}

	brokers, total, err := h.repo.Search(c.Request.Context(), tenantID, filter)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Brokers retrieved successfully", []dto.BrokerResponse{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Brokers retrieved successfully", dto.MapBrokersToDTO(brokers), page, int(limit), total)
}

func (h *BrokerHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	// SECURITY (achado A19): leitura tenant-scoped. Antes a query era só por id
	// e devolvia PII de corretor de qualquer tenant.
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}
	b, err := h.repo.GetByID(c.Request.Context(), tenantID, id)
	if err != nil || b == nil {
		response.NotFound(c, "broker not found")
		return
	}
	response.Success(c, "Broker retrieved successfully", dto.MapBrokerToDTO(b))
}

func (h *BrokerHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateBrokerRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	existing, err := h.repo.GetByID(c.Request.Context(), tenantID, id)
	if err != nil || existing == nil {
		response.NotFound(c, "broker not found")
		return
	}

	if existing.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}

	// Snapshot before-state so we can emit broker.updated only when a
	// user-visible field actually changed (avoids spamming the bus for
	// pause/active toggles that downstream snapshot consumers don't care
	// about).
	prevName := existing.Name
	prevRole := existing.Role
	prevEmail := existing.Email
	prevAvatar := existing.Avatar

	changed := false

	if req.Name != nil {
		existing.Name = *req.Name
		changed = true
	}
	if req.Role != nil {
		existing.Role = *req.Role
		changed = true
	}
	if req.Email != nil {
		existing.Email = *req.Email
		changed = true
	}
	if req.Avatar != nil {
		existing.Avatar = *req.Avatar
		changed = true
	}
	if req.Active != nil {
		existing.Active = *req.Active
		changed = true
	}
	if req.MaxActiveLeads != nil {
		existing.MaxActiveLeads = *req.MaxActiveLeads
		changed = true
	}
	if req.Specialties != nil {
		existing.Specialties = req.Specialties
		changed = true
	}
	if req.Regions != nil {
		existing.Regions = req.Regions
		changed = true
	}
	if req.Weight != nil {
		existing.Weight = *req.Weight
		changed = true
	}
	if req.IsPriority != nil {
		existing.IsPriority = *req.IsPriority
		changed = true
	}
	if req.Paused != nil {
		existing.Paused = *req.Paused
		changed = true
	}
	if req.BusinessHours != nil {
		existing.BusinessHours = req.BusinessHours
		changed = true
	}

	if !changed {
		response.ValidationError(c, "no fields to update")
		return
	}

	if err := h.repo.Update(c.Request.Context(), existing); err != nil {
		response.InternalError(c, err)
		return
	}

	// Emit broker.updated so downstream services that snapshot broker
	// labels (Deal.BrokerParticipations in toca-deal, analytics) can
	// refresh their projections. Only fire when a label-relevant field
	// changed — operational toggles (paused/active) are part of the
	// payload for completeness but don't drive label refresh.
	if h.eventBus != nil && brokerLabelChanged(prevName, prevRole, prevEmail, prevAvatar, existing) {
		evt := contracts.BrokerUpdatedEvent{
			Data: contracts.BrokerUpdatedEventPayload{
				BrokerID:  existing.ID,
				TenantID:  existing.TenantID,
				Name:      existing.Name,
				Role:      existing.Role,
				Email:     existing.Email,
				Avatar:    existing.Avatar,
				Active:    existing.Active,
				Paused:    existing.Paused,
				UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			},
		}
		if err := h.eventBus.PublishReliable(c.Request.Context(), evt); err != nil {
			log.Printf("WARN: failed to enqueue broker.updated event for broker=%s: %v", existing.ID, err)
		}
	}

	response.Success(c, "Broker updated successfully", dto.MapBrokerToDTO(existing))
}

// brokerLabelChanged reports whether any of the snapshot-relevant fields
// were modified. The Update handler captured prev* before applying the
// patch — compare against the now-saved struct.
func brokerLabelChanged(prevName, prevRole, prevEmail, prevAvatar string, b *broker.Broker) bool {
	if b == nil {
		return false
	}
	return prevName != b.Name || prevRole != b.Role || prevEmail != b.Email || prevAvatar != b.Avatar
}

func (h *BrokerHandler) UpdateMyQualification(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	accountID, err := pkgcontext.GetAccountID(c.Request.Context())
	if err != nil || accountID == "" {
		response.Forbidden(c, "user context required")
		return
	}

	var req UpdateMyQualificationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// A19: o corretor se resolve no próprio tenant do token. Antes a busca era
	// só por account id (cross-tenant) e um papel sem broker no tenant podia
	// atingir o registro de outro tenant.
	selfTenant, terr := pkgcontext.GetTenantID(c.Request.Context())
	if terr != nil || selfTenant == "" {
		response.Forbidden(c, "tenant context required")
		return
	}
	existing, err := h.repo.GetByID(c.Request.Context(), selfTenant, accountID)
	if err != nil || existing == nil {
		response.NotFound(c, "broker not found")
		return
	}
	if existing.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}

	changed := false
	if req.Specialties != nil {
		existing.Specialties = req.Specialties
		changed = true
	}
	if req.Regions != nil {
		existing.Regions = req.Regions
		changed = true
	}
	if req.BusinessHours != nil {
		existing.BusinessHours = req.BusinessHours
		changed = true
	}
	if !changed {
		response.ValidationError(c, "no fields to update")
		return
	}

	if err := h.repo.Update(c.Request.Context(), existing); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Broker updated successfully", dto.MapBrokerToDTO(existing))
}

func (h *BrokerHandler) GetPerformance(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	b, err := h.repo.GetByID(c.Request.Context(), tenantID, id)
	if err != nil || b == nil {
		response.NotFound(c, "broker not found")
		return
	}
	if b.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}

	perf, err := h.repo.GetPerformance(c.Request.Context(), id, tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Broker performance retrieved successfully", perf)
}

func (h *BrokerHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	// Captura o estado anterior para que o evento de "remoção" preserve os labels
	// (downstream consumers podem precisar do nome/avatar para limpar projeções).
	existing, _ := h.repo.GetByID(c.Request.Context(), tenantID, id)
	if existing != nil && existing.TenantID != tenantID {
		existing = nil // safety: não vazar dados entre tenants no evento
	}

	if err := h.repo.Delete(c.Request.Context(), id, tenantID); err != nil {
		response.InternalError(c, err)
		return
	}

	// Sinaliza a remoção via BrokerUpdatedEvent com Active=false. Os clientes WebSocket
	// vão simplesmente refazer fetch — não precisamos de um EventName separado.
	if existing != nil {
		existing.Active = false
		h.publishBrokerChanged(c, existing)
	}

	response.Success(c, "Broker deleted successfully", nil)
}

// publishBrokerChanged emite o evento `broker.updated` para o RabbitMQ. Falha do publish
// é apenas logada — a mutação principal já foi commitada e os clientes têm o revalidate
// on focus como fallback se o broadcast não chegar.
func (h *BrokerHandler) publishBrokerChanged(c *gin.Context, b *broker.Broker) {
	if h.eventBus == nil || b == nil {
		return
	}
	evt := contracts.BrokerUpdatedEvent{
		Data: contracts.BrokerUpdatedEventPayload{
			BrokerID:  b.ID,
			TenantID:  b.TenantID,
			Name:      b.Name,
			Role:      b.Role,
			Email:     b.Email,
			Avatar:    b.Avatar,
			Active:    b.Active,
			Paused:    b.Paused,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := h.eventBus.PublishReliable(c.Request.Context(), evt); err != nil {
		log.Printf("WARN: failed to enqueue broker.updated event for broker=%s: %v", b.ID, err)
	}
}

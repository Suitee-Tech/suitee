package dto

import (
	"time"

	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// LeadResponse represents the DTO for lead.Lead
type LeadResponse struct {
	ID                 string                 `json:"id"`
	DisplayID          string                 `json:"display_id,omitempty"`
	Name               string                 `json:"name"`
	Email              string                 `json:"email"`
	Phone              string                 `json:"phone"`
	Avatar             string                 `json:"avatar,omitempty"`
	Source             string                 `json:"source"`
	Status             string                 `json:"status"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	LastInteraction    *time.Time             `json:"last_interaction,omitempty"`
	PropertyInterest   *lead.LeadProperty     `json:"property_interest,omitempty"`
	PropertyIDs        []string               `json:"property_ids,omitempty"`
	AssignedBrokerID   string                 `json:"assigned_broker_id,omitempty"`
	History            []LeadHistory          `json:"history,omitempty"`
	IsClient           bool                   `json:"is_client,omitempty"`
	Role               string                 `json:"role,omitempty"`
	AccountID          string                 `json:"account_id,omitempty"`
	CaptacaoPropertyID string                 `json:"captacao_property_id,omitempty"`
	OwnerLeadID        string                 `json:"owner_lead_id,omitempty"`
	ContractID         string                 `json:"contract_id,omitempty"`
	SLARetries         map[string]int         `json:"sla_retries,omitempty"`
	Qualification      map[string]interface{} `json:"qualification,omitempty"`
	// PartnershipShared — per-lead opt-in to the partner shared-content feature.
	PartnershipShared bool `json:"partnership_shared,omitempty"`
}

type LeadHistory struct {
	ID             string    `json:"id"`
	Action         string    `json:"action"`
	Timestamp      time.Time `json:"timestamp"`
	PreviousStatus *string   `json:"previous_status,omitempty"`
	NewStatus      *string   `json:"new_status,omitempty"`
	Details        string    `json:"details,omitempty"`
	UserID         string    `json:"user_id,omitempty"`
}

// BrokerResponse represents the DTO for broker.Broker
type BrokerResponse struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	Role           string                `json:"role"`
	Email          string                `json:"email,omitempty"`
	Avatar         string                `json:"avatar,omitempty"`
	Active         bool                  `json:"active"`
	MaxActiveLeads int                   `json:"max_active_leads,omitempty"`
	Specialties    []string              `json:"specialties,omitempty"`
	Regions        []string              `json:"regions,omitempty"`
	Weight         float64               `json:"weight,omitempty"`
	IsPriority     bool                  `json:"is_priority,omitempty"`
	Paused         bool                  `json:"paused,omitempty"`
	BusinessHours  *broker.BusinessHours `json:"business_hours,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

// RouletteConfigResponse represents the DTO for lead.RouletteConfig
type RouletteConfigResponse struct {
	Enabled             bool                    `json:"enabled"`
	ActiveBrokers       []string                `json:"active_brokers"`
	LastIndex           int                     `json:"last_index"`
	UpdatedAt           time.Time               `json:"updated_at"`
	UpdatedBy           string                  `json:"updatedBy,omitempty"`
	StageBrokerPools    map[string][]string     `json:"stage_broker_pools,omitempty"`
	StageLastIndex      map[string]int          `json:"stage_last_index,omitempty"`
	DistMethod          string                  `json:"distMethod"`
	RespectSpecialty    bool                    `json:"respectSpecialty"`
	RespectRegion       bool                    `json:"respectRegion"`
	RespectSchedule     bool                    `json:"respectSchedule"`
	AutoCapLimit        bool                    `json:"autoCapLimit"`
	SLATiers            []lead.SLATier          `json:"slaTiers"`
	Escalation          []lead.EscalationStep   `json:"escalation"`
	MaxEscalations      int                     `json:"maxEscalations,omitempty"`
	NotifChannels       []lead.NotifChannel     `json:"notifChannels"`
	QualFields          []lead.QualField        `json:"qualFields"`
	QualRouting         *lead.QualRoutingConfig `json:"qualRouting,omitempty"`
	GlobalBusinessHours *broker.BusinessHours   `json:"globalBusinessHours,omitempty"`
	SimulationSeed      int64                   `json:"simulationSeed,omitempty"`
	// Warnings descreve configurações que estão tecnicamente válidas mas
	// não vão ter o efeito esperado (ex: distMethod=weighted sem nenhum
	// broker com Weight > 0). O frontend exibe como banner para o admin
	// resolver antes que a roleta rode em silêncio. Wave P1.4 — fixa o
	// "silent misconfiguration trap" de weighted/priority.
	Warnings []string `json:"warnings,omitempty"`
}

// RouletteHistoryResponse represents the DTO for lead.RouletteHistory
type RouletteHistoryResponse struct {
	ID           string    `json:"id"`
	LeadID       string    `json:"lead_id"`
	FromBrokerID string    `json:"from_broker_id,omitempty"`
	ToBrokerID   string    `json:"to_broker_id"`
	Stage        string    `json:"stage"`
	Method       string    `json:"method"`
	Reason       string    `json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type RouletteAuditResponse struct {
	ID         string                 `json:"id"`
	ActorID    string                 `json:"actor_id"`
	ActorEmail string                 `json:"actor_email,omitempty"`
	Action     string                 `json:"action"`
	TargetID   string                 `json:"target_id,omitempty"`
	Diff       map[string]interface{} `json:"diff"`
	CreatedAt  time.Time              `json:"created_at"`
}

// Mappers

func MapLeadToDTO(l *lead.Lead) *LeadResponse {
	if l == nil {
		return nil
	}

	history := make([]LeadHistory, len(l.History))
	for i, h := range l.History {
		var prevStatus, newStatus *string
		if h.PreviousStatus != nil {
			s := string(*h.PreviousStatus)
			prevStatus = &s
		}
		if h.NewStatus != nil {
			s := string(*h.NewStatus)
			newStatus = &s
		}

		history[i] = LeadHistory{
			ID:             h.ID,
			Action:         string(h.Action),
			Timestamp:      h.Timestamp,
			PreviousStatus: prevStatus,
			NewStatus:      newStatus,
			Details:        h.Details,
			UserID:         h.UserID,
		}
	}

	return &LeadResponse{
		ID:                 l.ID,
		DisplayID:          l.DisplayID,
		Name:               l.Name,
		Email:              l.Email,
		Phone:              l.Phone,
		Avatar:             l.Avatar,
		Source:             string(l.Source),
		Status:             string(l.Status),
		CreatedAt:          l.CreatedAt,
		UpdatedAt:          l.UpdatedAt,
		LastInteraction:    l.LastInteraction,
		PropertyInterest:   l.PropertyInterest,
		PropertyIDs:        l.PropertyIDs,
		AssignedBrokerID:   l.AssignedBrokerID,
		History:            history,
		IsClient:           l.IsClient,
		Role:               string(l.Role),
		AccountID:          l.AccountID,
		CaptacaoPropertyID: l.CaptacaoPropertyID,
		OwnerLeadID:        l.OwnerLeadID,
		ContractID:         l.ContractID,
		SLARetries:         l.SLARetries,
		Qualification:      l.Qualification,
		PartnershipShared:  l.PartnershipShared,
	}
}

func MapLeadsToDTO(leads []*lead.Lead) []LeadResponse {
	dtos := make([]LeadResponse, len(leads))
	for i, l := range leads {
		dtos[i] = *MapLeadToDTO(l)
	}
	return dtos
}

func MapBrokerToDTO(b *broker.Broker) *BrokerResponse {
	if b == nil {
		return nil
	}
	return &BrokerResponse{
		ID:             b.ID,
		Name:           b.Name,
		Role:           b.Role,
		Email:          b.Email,
		Avatar:         b.Avatar,
		Active:         b.Active,
		MaxActiveLeads: b.MaxActiveLeads,
		Specialties:    b.Specialties,
		Regions:        b.Regions,
		Weight:         b.Weight,
		IsPriority:     b.IsPriority,
		Paused:         b.Paused,
		BusinessHours:  b.BusinessHours,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}
}

func MapBrokersToDTO(brokers []*broker.Broker) []BrokerResponse {
	dtos := make([]BrokerResponse, len(brokers))
	for i, b := range brokers {
		dtos[i] = *MapBrokerToDTO(b)
	}
	return dtos
}

func MapRouletteConfigToDTO(r *lead.RouletteConfig) *RouletteConfigResponse {
	if r == nil {
		return nil
	}
	return &RouletteConfigResponse{
		Enabled:             r.Enabled,
		ActiveBrokers:       r.ActiveBrokers,
		LastIndex:           r.LastIndex,
		UpdatedAt:           r.UpdatedAt,
		UpdatedBy:           r.UpdatedBy,
		StageBrokerPools:    r.StageBrokerPools,
		StageLastIndex:      r.StageLastIndex,
		DistMethod:          string(r.DistMethod),
		RespectSpecialty:    r.RespectSpecialty,
		RespectRegion:       r.RespectRegion,
		RespectSchedule:     r.RespectSchedule,
		AutoCapLimit:        r.AutoCapLimit,
		SLATiers:            r.SLATiers,
		Escalation:          r.Escalation,
		MaxEscalations:      r.MaxEscalations,
		NotifChannels:       r.NotifChannels,
		QualFields:          r.QualFields,
		QualRouting:         r.QualRouting,
		GlobalBusinessHours: r.GlobalBusinessHours,
		SimulationSeed:      r.SimulationSeed,
	}
}

func MapRouletteHistoryToDTO(h *lead.RouletteHistory) *RouletteHistoryResponse {
	if h == nil {
		return nil
	}
	return &RouletteHistoryResponse{
		ID:           h.ID,
		LeadID:       h.LeadID,
		FromBrokerID: h.FromBrokerID,
		ToBrokerID:   h.ToBrokerID,
		Stage:        h.Stage,
		Method:       h.Method,
		Reason:       h.Reason,
		CreatedAt:    h.CreatedAt,
	}
}

func MapRouletteHistoriesToDTO(histories []*lead.RouletteHistory) []RouletteHistoryResponse {
	dtos := make([]RouletteHistoryResponse, len(histories))
	for i, h := range histories {
		dtos[i] = *MapRouletteHistoryToDTO(h)
	}
	return dtos
}

func MapRouletteAuditToDTO(e *lead.RouletteAuditEntry) *RouletteAuditResponse {
	if e == nil {
		return nil
	}
	return &RouletteAuditResponse{
		ID:         e.ID,
		ActorID:    e.ActorID,
		ActorEmail: e.ActorEmail,
		Action:     e.Action,
		TargetID:   e.TargetID,
		Diff:       e.Diff,
		CreatedAt:  e.CreatedAt,
	}
}

func MapRouletteAuditsToDTO(items []*lead.RouletteAuditEntry) []RouletteAuditResponse {
	dtos := make([]RouletteAuditResponse, len(items))
	for i, e := range items {
		dtos[i] = *MapRouletteAuditToDTO(e)
	}
	return dtos
}

package lead

import (
	"time"

	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
)

type DistMethod string

// DefaultMaxEscalations is the cap used when RouletteConfig.MaxEscalations is
// zero or unset. After this many escalation steps a lead is marked LOST.
const DefaultMaxEscalations = 3

const (
	DistMethodSequential  DistMethod = "sequential"
	DistMethodLoadBalance DistMethod = "load_balance"
	DistMethodWeighted    DistMethod = "weighted"
	DistMethodPriority    DistMethod = "priority"
)

type SLATierActions struct {
	RedirectNext    bool `bson:"redirect_next" json:"redirectNext"`
	AutoPauseBroker bool `bson:"auto_pause_broker" json:"autoPauseBroker"`
	// AutoPauseCooldownMin define quantos minutos o broker fica auto-pausado
	// antes de voltar à roleta sem intervenção manual. Zero ou negativo
	// significa "use o default do domínio" (broker.DefaultBrokerPauseCooldown).
	// Wave P1.7 — pausa indefinida virou seam crítica em plantão.
	AutoPauseCooldownMin int  `bson:"auto_pause_cooldown_min,omitempty" json:"autoPauseCooldownMin,omitempty"`
	NotifyManager        bool `bson:"notify_manager" json:"notifyManager"`
	EscalateToSupervisor bool `bson:"escalate_to_supervisor" json:"escalateToSupervisor"`
	MarkAsUrgent         bool `bson:"mark_as_urgent" json:"markAsUrgent"`
}

type SLATier struct {
	ID           string         `bson:"id" json:"id"`
	Name         string         `bson:"name" json:"name"`
	Color        string         `bson:"color" json:"color"`
	TimeLimitMin int            `bson:"time_limit_min" json:"timeLimitMin"`
	MaxRetries   int            `bson:"max_retries" json:"maxRetries"`
	Actions      SLATierActions `bson:"actions" json:"actions"`
	AppliesTo    []string       `bson:"applies_to" json:"appliesTo"`
}

type EscalationStep struct {
	ID           string `bson:"id" json:"id"`
	AfterMinutes int    `bson:"after_minutes" json:"afterMinutes"`
	Action       string `bson:"action" json:"action"`
	Label        string `bson:"label" json:"label"`
}

type NotifChannel struct {
	ID          string `bson:"id" json:"id"`
	Name        string `bson:"name" json:"name"`
	Icon        string `bson:"icon" json:"icon"`
	Enabled     bool   `bson:"enabled" json:"enabled"`
	Description string `bson:"description" json:"description"`
}

type QualField struct {
	ID               string   `bson:"id" json:"id"`
	Label            string   `bson:"label" json:"label"`
	Type             string   `bson:"type" json:"type"`
	Required         bool     `bson:"required" json:"required"`
	Active           bool     `bson:"active" json:"active"`
	Options          []string `bson:"options" json:"options"`
	Placeholder      string   `bson:"placeholder" json:"placeholder"`
	HelpText         string   `bson:"help_text" json:"helpText"`
	ConditionalOn    *string  `bson:"conditional_on" json:"conditionalOn"`
	ConditionalValue *string  `bson:"conditional_value" json:"conditionalValue"`
}

type QualRoutingConfig struct {
	SpecialtyFieldIDs []string `bson:"specialty_field_ids,omitempty" json:"specialtyFieldIds"`
	RegionFieldID     *string  `bson:"region_field_id,omitempty" json:"regionFieldId"`
}

type RouletteConfig struct {
	TenantID      string    `bson:"tenant_id"`
	Enabled       bool      `bson:"enabled"`
	ActiveBrokers []string  `bson:"active_brokers"`
	LastIndex     int       `bson:"last_index"`
	UpdatedAt     time.Time `bson:"updated_at"`
	UpdatedBy     string    `bson:"updated_by,omitempty"`

	// Esteira (pipeline) → pool de corretores por etapa/status do lead.
	// Se uma etapa não estiver configurada (ou estiver vazia), cai no fallback ActiveBrokers.
	StageBrokerPools map[string][]string `bson:"stage_broker_pools,omitempty"`
	// Índice de round-robin por etapa/status (para não misturar a rotação entre etapas).
	StageLastIndex map[string]int `bson:"stage_last_index,omitempty"`

	// Rules & SLA
	DistMethod       DistMethod       `bson:"dist_method"`
	RespectSpecialty bool             `bson:"respect_specialty"`
	RespectRegion    bool             `bson:"respect_region"`
	RespectSchedule  bool             `bson:"respect_schedule"`
	AutoCapLimit     bool             `bson:"auto_cap_limit"`
	SLATiers         []SLATier        `bson:"sla_tiers"`
	Escalation       []EscalationStep `bson:"escalation"`
	// MaxEscalations caps how many escalation steps a single lead can go through
	// before being marked LOST. Zero or negative is treated as DefaultMaxEscalations.
	MaxEscalations int            `bson:"max_escalations,omitempty"`
	NotifChannels  []NotifChannel `bson:"notif_channels"`

	// Qualification Forms
	QualFields  []QualField        `bson:"qual_fields"`
	QualRouting *QualRoutingConfig `bson:"qual_routing,omitempty"`

	GlobalBusinessHours *broker.BusinessHours `bson:"global_business_hours,omitempty"`
	SimulationSeed      int64                 `bson:"simulation_seed,omitempty"`
}

type UpdateRouletteConfigDTO struct {
	Enabled               *bool               `json:"enabled"`
	AssignUnassignedLeads *bool               `json:"assign_unassigned_leads,omitempty"`
	ActiveBrokers         []string            `json:"active_brokers"`
	StageBrokerPools      map[string][]string `json:"stage_broker_pools,omitempty"`

	DistMethod       *DistMethod      `json:"distMethod"`
	RespectSpecialty *bool            `json:"respectSpecialty"`
	RespectRegion    *bool            `json:"respectRegion"`
	RespectSchedule  *bool            `json:"respectSchedule"`
	AutoCapLimit     *bool            `json:"autoCapLimit"`
	SLATiers         []SLATier        `json:"slaTiers"`
	Escalation       []EscalationStep `json:"escalation"`
	MaxEscalations   *int             `json:"maxEscalations,omitempty"`
	NotifChannels    []NotifChannel   `json:"notifChannels"`

	QualFields  []QualField        `json:"qualFields"`
	QualRouting *QualRoutingConfig `json:"qualRouting,omitempty"`

	GlobalBusinessHours *broker.BusinessHours `json:"globalBusinessHours,omitempty"`
	SimulationSeed      *int64                `json:"simulationSeed,omitempty"`
}

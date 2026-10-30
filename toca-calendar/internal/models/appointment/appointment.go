package appointment

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPendente    Status = "Pendente"
	StatusReagendado  Status = "Reagendado"
	StatusAgendado    Status = "Agendado"
	StatusConfirmado  Status = "Confirmado"
	StatusEmAndamento Status = "Em Andamento"
	StatusConcluido   Status = "Concluido"
	StatusCancelado   Status = "Cancelado"
)

type Type string

const (
	TypeVisita     Type = "Visita"
	TypeReuniao    Type = "Reuniao"
	TypeVistoria   Type = "Vistoria"
	TypeAssinatura Type = "Assinatura"
	TypeEntrega    Type = "Entrega"
)

type Priority string

type Visibility string

const (
	PriorityAlta  Priority = "Alta"
	PriorityMedia Priority = "Media"
	PriorityBaixa Priority = "Baixa"
)

const (
	VisibilityTeam    Visibility = "team"
	VisibilityPrivate Visibility = "private"
)

type Appointment struct {
	InspectorName        string    `json:"inspector_name,omitempty"`
	InspectionID         string    `json:"inspection_id,omitempty"`
	InspectionContractID string    `json:"inspection_contract_id,omitempty"`
	InspectorID          string    `json:"inspector_id,omitempty"`
	SourceUpdatedAt      time.Time `json:"source_updated_at,omitempty"`

	ID                string     `json:"id" bson:"_id"`
	DisplayID         string     `json:"display_id" bson:"display_id"`
	TenantID          string     `json:"tenant_id" bson:"tenant_id"`
	Title             string     `json:"title" bson:"title"`
	Date              string     `json:"date" bson:"date"`         // YYYY-MM-DD
	Time              string     `json:"time" bson:"time"`         // HH:MM
	EndTime           string     `json:"end_time" bson:"end_time"` // HH:MM
	Type              Type       `json:"type" bson:"type"`
	Priority          Priority   `json:"priority" bson:"priority"`
	Client            string     `json:"client" bson:"client"`
	LeadID            string     `json:"lead_id,omitempty" bson:"lead_id,omitempty"`
	Property          string     `json:"property" bson:"property"`
	PropertyID        string     `json:"property_id,omitempty" bson:"property_id,omitempty"`
	Agent             string     `json:"agent" bson:"agent"`
	AgentID           string     `json:"agent_id,omitempty" bson:"agent_id,omitempty"`
	DealID            string     `json:"deal_id,omitempty" bson:"deal_id,omitempty"`
	CustomerAccountID string     `json:"customer_account_id,omitempty" bson:"customer_account_id,omitempty"`
	Visibility        Visibility `json:"visibility,omitempty" bson:"visibility,omitempty"`
	Status            Status     `json:"status" bson:"status"`
	Description       string     `json:"description,omitempty" bson:"description,omitempty"`
	CreatedAt         time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" bson:"updated_at"`

	// CalendarURL é o link assinado do arquivo .ics. Vive só no payload dos
	// eventos de agenda (a comunicação insere no e-mail); nunca é persistido.
	CalendarURL string `json:"calendar_url,omitempty" bson:"-"`
}

func New(tenantID, title, date, startTime, endTime string, apptType Type, priority Priority, client, agent string) *Appointment {
	now := time.Now()
	return &Appointment{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		Title:     title,
		Date:      date,
		Time:      startTime,
		EndTime:   endTime,
		Type:      apptType,
		Priority:  priority,
		Client:    client,
		Agent:     agent,
		Status:    StatusAgendado,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

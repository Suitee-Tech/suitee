package inspection

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/events/contracts"
)

const (
	StatusAgendada    = "Agendada"
	StatusEmAndamento = "Em Andamento"
	StatusConcluida   = "Concluída"
	StatusCancelada   = "Cancelada"
)

const (
	ConditionBom     = "Bom"
	ConditionRegular = "Regular"
	ConditionRuim    = "Ruim"
)

const (
	TypeEntry    = "ENTRY"
	TypeExit     = "EXIT"
	TypePeriodic = "PERIODIC"
)

var typeAliases = map[string]string{
	"entrada":   TypeEntry,
	"entry":     TypeEntry,
	"saida":     TypeExit,
	"saída":     TypeExit,
	"exit":      TypeExit,
	"periodic":  TypePeriodic,
	"periodica": TypePeriodic,
	"periódica": TypePeriodic,
}

func NormalizeType(t string) string {
	if t == "" {
		return ""
	}
	if mapped, ok := typeAliases[strings.ToLower(strings.TrimSpace(t))]; ok {
		return mapped
	}
	return strings.ToUpper(strings.TrimSpace(t))
}

func ValidateType(t string) error {
	switch NormalizeType(t) {
	case TypeEntry, TypeExit, TypePeriodic:
		return nil
	}
	return fmt.Errorf("invalid inspection type %q: expected ENTRY, EXIT or PERIODIC", t)
}

type Finding struct {
	ID          string   `json:"id" bson:"id"`
	Room        string   `json:"room" bson:"room"`
	Description string   `json:"description" bson:"description"`
	Condition   string   `json:"condition" bson:"condition"`
	Notes       string   `json:"notes,omitempty" bson:"notes,omitempty"`
	Photos      []string `json:"photos" bson:"photos"`
	Thumbnails  []string `json:"thumbnails,omitempty" bson:"thumbnails,omitempty"`
}

func EnsureIDs(findings []Finding) []Finding {
	out := make([]Finding, len(findings))
	copy(out, findings)
	for i := range out {
		if out[i].ID == "" {
			out[i].ID = uuid.New().String()
		}
	}
	return out
}

func FindFindingIndex(findings []Finding, id string) int {
	for i := range findings {
		if findings[i].ID == id {
			return i
		}
	}
	return -1
}

type Deduction struct {
	Room           string   `json:"room" bson:"room"`
	Description    string   `json:"description" bson:"description"`
	Amount         float64  `json:"amount" bson:"amount"`
	EvidencePhotos []string `json:"evidence_photos,omitempty" bson:"evidence_photos,omitempty"`
}

type Inspection struct {
	TimeZone      string      `json:"time_zone,omitempty"`
	ScheduledBy   string      `json:"scheduled_by,omitempty"`
	ID            string      `json:"id" bson:"_id"`
	TenantID      string      `json:"tenant_id" bson:"tenant_id"`
	PropertyID    string      `json:"property_id" bson:"property_id"`
	ContractID    string      `json:"contract_id" bson:"contract_id"`
	Type          string      `json:"type" bson:"type"`
	ScheduledDate time.Time   `json:"scheduled_date" bson:"scheduled_date"`
	CompletedDate *time.Time  `json:"completed_date,omitempty" bson:"completed_date,omitempty"`
	InspectorID   string      `json:"inspector_id" bson:"inspector_id"`
	InspectorName string      `json:"inspector_name" bson:"inspector_name"`
	Status        string      `json:"status" bson:"status"`
	Findings      []Finding   `json:"findings" bson:"findings"`
	Notes         string      `json:"notes" bson:"notes"`
	ReportURL     string      `json:"report_url,omitempty" bson:"report_url,omitempty"`
	Deductions    []Deduction `json:"deductions,omitempty" bson:"deductions,omitempty"`
	CreatedAt     time.Time   `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at" bson:"updated_at"`
}

func New(propertyID, contractID, inspType, inspectorID, inspectorName, tenantID string, scheduledDate time.Time) *Inspection {
	now := time.Now()
	return &Inspection{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		PropertyID:    propertyID,
		ContractID:    contractID,
		Type:          NormalizeType(inspType),
		ScheduledDate: scheduledDate,
		InspectorID:   inspectorID,
		InspectorName: inspectorName,
		Status:        StatusAgendada,
		Findings:      []Finding{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// CalendarEvent is persisted alongside the inspection, including cancellation.
func (i *Inspection) CalendarEvent() contracts.InspectionCalendarEvent {
	return contracts.InspectionCalendarEvent{ID: uuid.NewString(), Data: contracts.InspectionCalendarPayload{
		InspectionID: i.ID, TenantID: i.TenantID, ContractID: i.ContractID, PropertyID: i.PropertyID,
		Type: i.Type, ScheduledDate: i.ScheduledDate, TimeZone: i.TimeZone, ScheduledBy: i.ScheduledBy,
		InspectorID: i.InspectorID, InspectorName: i.InspectorName, Status: i.Status, UpdatedAt: i.UpdatedAt,
	}}
}

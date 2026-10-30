package contracts

import "github.com/google/uuid"

// InspectionScheduledEventPayload — published by toca-ops the moment a
// vistoria is created (Status=Agendada). Subscribers:
// - toca-deal: pin the ENTRY inspection id on the Contract so the UI can
//   show "vistoria de entrada pendente" without an extra query.
// - toca-comm: notify the assigned inspector + the tenant of the date.
type InspectionScheduledEventPayload struct {
	InspectionID  string `json:"inspection_id"`
	TenantID      string `json:"tenant_id"`
	PropertyID    string `json:"property_id"`
	ContractID    string `json:"contract_id"`
	Type          string `json:"type"` // ENTRY | EXIT | PERIODIC
	ScheduledDate string `json:"scheduled_date"`
	InspectorID   string `json:"inspector_id,omitempty"`
	Timestamp     string `json:"timestamp"`
}

type InspectionScheduledEvent struct {
	EventIDVal string                          `json:"event_id"`
	Data       InspectionScheduledEventPayload `json:"payload"`
}

func (e InspectionScheduledEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e InspectionScheduledEvent) Name() string         { return "inspection.scheduled" }
func (e InspectionScheduledEvent) Payload() interface{} { return e.Data }

// InspectionCompletedEventPayload — fires when a vistoria moves to
// Concluída with a ReportURL set. The completed event is the trigger for:
// - toca-deal: pin the EXIT inspection id (closing the termination gate)
//   and capture a Document of category "laudo" in FASE 3.
// - toca-comm: send the laudo PDF link to both parties.
//
// HasDamage is a derived flag (Findings include at least one
// Condition!="Bom") used by the financial side in FASE 4 to gate the
// security-deposit deduction flow.
type InspectionCompletedEventPayload struct {
	InspectionID  string `json:"inspection_id"`
	TenantID      string `json:"tenant_id"`
	PropertyID    string `json:"property_id"`
	ContractID    string `json:"contract_id"`
	Type          string `json:"type"`
	CompletedAt   string `json:"completed_at"`
	ReportURL     string `json:"report_url"`
	FindingsCount int    `json:"findings_count"`
	HasDamage     bool   `json:"has_damage"`
	Timestamp     string `json:"timestamp"`
}

type InspectionCompletedEvent struct {
	EventIDVal string                          `json:"event_id"`
	Data       InspectionCompletedEventPayload `json:"payload"`
}

func (e InspectionCompletedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e InspectionCompletedEvent) Name() string         { return "inspection.completed" }
func (e InspectionCompletedEvent) Payload() interface{} { return e.Data }

// InspectionCancelledEventPayload — fires when a vistoria moves to
// Cancelada (status), via Update. Issue #439. Subscribers:
// - toca-deal: clear the pinned inspection_id on Contract so a new
//   one pode ser agendada
// - toca-comm: notificar partes que a vistoria foi cancelada e que
//   uma nova será agendada
//
// Reason é opcional — caller pode enviar via Notes ao cancelar e o
// service relay aqui; quando vazio, frontend mostra "Cancelada pelo
// operador" sem detalhe.
type InspectionCancelledEventPayload struct {
	InspectionID string `json:"inspection_id"`
	TenantID     string `json:"tenant_id"`
	PropertyID   string `json:"property_id"`
	ContractID   string `json:"contract_id"`
	Type         string `json:"type"`
	Reason       string `json:"reason,omitempty"`
	CancelledAt  string `json:"cancelled_at"`
	Timestamp    string `json:"timestamp"`
}

type InspectionCancelledEvent struct {
	EventIDVal string                          `json:"event_id"`
	Data       InspectionCancelledEventPayload `json:"payload"`
}

func (e InspectionCancelledEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e InspectionCancelledEvent) Name() string         { return "inspection.cancelled" }
func (e InspectionCancelledEvent) Payload() interface{} { return e.Data }

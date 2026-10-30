package contracts

import "github.com/google/uuid"

// AppointmentUpdatedEventPayload carrega os campos necessários para notificar
// as partes (cliente do portal e corretor) quando um compromisso muda.
// AgentID e Title foram acrescentados no beta de e-mail transacional
// (notificacoes-email-beta.md, F2): sem eles o corretor nunca era notificado
// e a copy do e-mail ficava genérica. Aditivo — consumidores antigos ignoram.
type AppointmentUpdatedEventPayload struct {
	AppointmentID     string `json:"appointment_id"`
	DealID            string `json:"deal_id"`
	TenantID          string `json:"tenant_id"`
	Status            string `json:"status"`
	Date              string `json:"date"`
	Time              string `json:"time,omitempty"`
	EndTime           string `json:"end_time,omitempty"`
	LeadID            string `json:"lead_id,omitempty"`
	CustomerAccountID string `json:"customer_account_id"`
	AgentID           string `json:"agent_id,omitempty"`
	Title             string `json:"title,omitempty"`
	// CalendarURL é o link assinado do .ics, produzido pelo serviço de agenda
	// (que conhece a identidade do compromisso). A comunicação apenas o insere
	// no e-mail; vazio quando o compromisso não gera arquivo.
	CalendarURL string `json:"calendar_url,omitempty"`
}

type AppointmentUpdatedEvent struct {
	EventIDVal string                         `json:"event_id"`
	Data       AppointmentUpdatedEventPayload `json:"payload"`
}

func (e AppointmentUpdatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e AppointmentUpdatedEvent) Name() string { return "appointment.updated" }

func (e AppointmentUpdatedEvent) Payload() interface{} { return e.Data }

// AppointmentCancelledEventPayload — AgentID/Title/Date/Time acrescentados no
// beta de e-mail transacional (F2) para que o corretor seja notificado e o
// e-mail de cancelamento mostre qual compromisso era. Aditivo.
type AppointmentCancelledEventPayload struct {
	AppointmentID     string `json:"appointment_id"`
	DealID            string `json:"deal_id"`
	TenantID          string `json:"tenant_id"`
	Reason            string `json:"reason"`
	CustomerAccountID string `json:"customer_account_id"`
	AgentID           string `json:"agent_id,omitempty"`
	Title             string `json:"title,omitempty"`
	Date              string `json:"date,omitempty"`
	Time              string `json:"time,omitempty"`
}

type AppointmentCancelledEvent struct {
	EventIDVal string                           `json:"event_id"`
	Data       AppointmentCancelledEventPayload `json:"payload"`
}

func (e AppointmentCancelledEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e AppointmentCancelledEvent) Name() string { return "appointment.cancelled" }

func (e AppointmentCancelledEvent) Payload() interface{} { return e.Data }

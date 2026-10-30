package contracts

import "github.com/google/uuid"

// MaintenanceRequestedEventPayload — published by toca-ops the moment a
// MaintenanceTicket is created. Subscribers in this codebase:
// - toca-financial: maybe auto-create a Charge for the tenant or owner
//   (depends on tenant policy).
// - toca-comm: ping the assigned broker / property owner via in-app
//   notification.
type MaintenanceRequestedEventPayload struct {
	TicketID        string `json:"ticket_id"`
	TenantID        string `json:"tenant_id"`
	PropertyID      string `json:"property_id"`
	ContractID      string `json:"contract_id,omitempty"`
	RentalID        string `json:"rental_id,omitempty"`
	RequesterLeadID string `json:"requester_lead_id,omitempty"`
	Category        string `json:"category"`
	Priority        string `json:"priority"`
	Description     string `json:"description"`
	EstimatedCost   float64 `json:"estimated_cost"`
	// PayerRole tells downstream charge handlers who absorbs the cost.
	// Common values: "owner", "tenant", "shared". Tenants set their default
	// per category via the financial settings; the field is best-effort
	// and may be empty if not configured (in which case toca-financial
	// won't auto-charge).
	PayerRole string `json:"payer_role,omitempty"`
	// BillingTarget escolhe ONDE o custo entra: "next_charge" (próxima
	// cobrança em aberto do contrato), "next_transfer" (próximo repasse
	// pendente do proprietário) ou "standalone" (cobrança avulsa). Vazio
	// mantém o comportamento derivado do payer_role.
	BillingTarget string `json:"billing_target,omitempty"`
	// AdvancedBy diz quem pagou o fornecedor: "imobiliaria" (default),
	// "proprietario" ou "inquilino". Define a natureza do valor — o custo é de
	// terceiro, e a imobiliária só adianta e recebe de volta.
	AdvancedBy string `json:"advanced_by,omitempty"`
	Timestamp     string `json:"timestamp"`
}

type MaintenanceRequestedEvent struct {
	EventIDVal string                           `json:"event_id"`
	Data       MaintenanceRequestedEventPayload `json:"payload"`
}

func (e MaintenanceRequestedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e MaintenanceRequestedEvent) Name() string         { return "maintenance.requested" }
func (e MaintenanceRequestedEvent) Payload() interface{} { return e.Data }

// MaintenanceCompletedEventPayload — fires when AssignedTo marks the ticket
// as Concluído. Triggers final charge reconciliation if `actual_cost`
// differs from the original `estimated_cost`.
//
// ContractID / PropertyID / Category / PayerRole / RequesterLeadID / EstimatedCost
// mirror what was sent in the matching MaintenanceRequestedEvent so the
// completion handler can reconcile without re-fetching the original ticket.
// All new fields are optional for backwards-compat with older publishers.
type MaintenanceCompletedEventPayload struct {
	TicketID        string  `json:"ticket_id"`
	TenantID        string  `json:"tenant_id"`
	ContractID      string  `json:"contract_id,omitempty"`
	RentalID        string  `json:"rental_id,omitempty"`
	PropertyID      string  `json:"property_id,omitempty"`
	RequesterLeadID string  `json:"requester_lead_id,omitempty"`
	Category        string  `json:"category,omitempty"`
	PayerRole       string  `json:"payer_role,omitempty"`
	// BillingTarget espelha o destino escolhido na requisição (next_charge,
	// next_transfer ou standalone).
	BillingTarget string `json:"billing_target,omitempty"`
	// AdvancedBy espelha quem pagou o fornecedor na requisição.
	AdvancedBy string  `json:"advanced_by,omitempty"`
	EstimatedCost float64 `json:"estimated_cost,omitempty"`
	ActualCost    float64 `json:"actual_cost"`
	CompletedAt     string  `json:"completed_at"`
	Timestamp       string  `json:"timestamp"`
}

type MaintenanceCompletedEvent struct {
	EventIDVal string                           `json:"event_id"`
	Data       MaintenanceCompletedEventPayload `json:"payload"`
}

func (e MaintenanceCompletedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e MaintenanceCompletedEvent) Name() string         { return "maintenance.completed" }
func (e MaintenanceCompletedEvent) Payload() interface{} { return e.Data }

package maintenance

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusEmAnalise   = "Em Análise"
	StatusEmAndamento = "Em Andamento"
	StatusConcluido   = "Concluído"
	StatusCancelado   = "Cancelado"
)

type MaintenanceTicket struct {
	ID         string `json:"id" bson:"_id"`
	TenantID   string `json:"tenant_id" bson:"tenant_id"`
	Type       string `json:"type" bson:"type"`
	TicketType string `json:"ticket_type" bson:"ticket_type"`
	Category   string `json:"category" bson:"category"`
	PropertyID string `json:"property_id" bson:"property_id"`
	ContractID string `json:"contract_id" bson:"contract_id"`
	RentalID   string `json:"rental_id,omitempty" bson:"rental_id,omitempty"`

	RequesterLeadID string   `json:"requester_lead_id,omitempty" bson:"requester_lead_id,omitempty"`
	PropertyTitle   string   `json:"property_title" bson:"property_title"`
	Requester       string   `json:"requester" bson:"requester"`
	Profile         string   `json:"profile" bson:"profile"`
	Status          string   `json:"status" bson:"status"`
	Priority        string   `json:"priority" bson:"priority"`
	Description     string   `json:"description" bson:"description"`
	Attachments     []string `json:"attachments,omitempty" bson:"attachments,omitempty"`
	AssignedTo      string   `json:"assigned_to" bson:"assigned_to"`
	// SupplierID aponta para o profissional/fornecedor cadastrado. Guardar a
	// referência (e não só o nome) permite reutilizar o mesmo prestador e
	// manter o histórico ligado ao cadastro.
	SupplierID string `json:"supplier_id,omitempty" bson:"supplier_id,omitempty"`
	// Instructions são as instruções passadas ao fornecedor/vistoriador.
	Instructions string `json:"instructions,omitempty" bson:"instructions,omitempty"`
	// ScheduledAt é a data e hora combinadas com o prestador.
	ScheduledAt   *time.Time `json:"scheduled_at,omitempty" bson:"scheduled_at,omitempty"`
	EstimatedCost float64    `json:"estimated_cost" bson:"estimated_cost"`
	ActualCost    float64    `json:"actual_cost" bson:"actual_cost"`
	PayerRole     string     `json:"payer_role,omitempty" bson:"payer_role,omitempty"`
	// BillingTarget define onde o custo entra no financeiro: "next_charge",
	// "next_transfer" ou "standalone". Vazio cai no padrão do payer_role.
	BillingTarget string `json:"billing_target,omitempty" bson:"billing_target,omitempty"`
	// AdvancedBy registra quem pagou o fornecedor: "imobiliaria" (default),
	// "proprietario" ou "inquilino". É o dado que define a natureza do valor
	// no financeiro — custo de terceiro adiantado, não receita da imobiliária.
	AdvancedBy  string     `json:"advanced_by,omitempty" bson:"advanced_by,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty" bson:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" bson:"updated_at"`
}

func New(ticketType, category, propertyID, contractID, propertyTitle, requester, profile, priority, description, tenantID string) *MaintenanceTicket {
	now := time.Now()
	return &MaintenanceTicket{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		Type:          ticketType,
		Category:      category,
		PropertyID:    propertyID,
		ContractID:    contractID,
		PropertyTitle: propertyTitle,
		Requester:     requester,
		Profile:       profile,
		Status:        StatusEmAnalise,
		Priority:      priority,
		Description:   description,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func NewWithRefs(ticketType, category, propertyID, contractID, rentalID, requesterLeadID, propertyTitle, requester, profile, priority, description, tenantID string) *MaintenanceTicket {
	t := New(ticketType, category, propertyID, contractID, propertyTitle, requester, profile, priority, description, tenantID)
	t.RentalID = rentalID
	t.RequesterLeadID = requesterLeadID
	return t
}
